package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runServer(subcmd string, args []string) error {
	cfg := DefaultConfig()

	// Verify docker-compose.yml exists
	if !fileExists(cfg.DockerComposeFile) {
		return fmt.Errorf("mithril not initialized — run 'mithril init' first")
	}

	switch subcmd {
	case "start":
		return serverStart(cfg)
	case "stop":
		return serverStop(cfg)
	case "restart":
		return serverRestart(cfg)
	case "rebuild":
		return serverRebuild(cfg)
	case "status":
		return serverStatus(cfg)
	case "attach":
		return serverAttach(cfg)
	case "logs":
		return serverLogs(cfg)
	case "account":
		if len(args) < 1 {
			return fmt.Errorf("server account requires a subcommand: create")
		}
		return runAccount(args[0], args[1:])
	default:
		return fmt.Errorf("unknown server subcommand: %s (use start, stop, restart, rebuild, status, attach, logs, account)", subcmd)
	}
}

func serverStart(cfg *Config) error {
	printInfo("Starting Mithril TrinityCore server...")

	// Check if data directory has maps
	if !fileExists(filepath.Join(cfg.DataDir, "maps")) {
		printWarning("No extracted map data found at " + cfg.DataDir)
		printWarning("The worldserver will fail to start without map data.")
		printInfo("Run 'mithril init' to set up the client and extract data.")
		fmt.Println()
	}

	if err := dockerCompose(cfg, "up", "-d"); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Brief health check: give the container a few seconds, then verify it
	// is still running and hasn't entered a restart loop.
	time.Sleep(3 * time.Second)
	containerID, err := composeContainerID(cfg)
	if err == nil && containerID != "" {
		state, err := containerState(containerID)
		if err == nil && state != "running" {
			fmt.Println()
			printWarning(fmt.Sprintf("Container state is '%s' — it may be crash-looping.", state))
			printInfo("Check the logs for errors:  mithril server logs")
			return fmt.Errorf("server failed to start (container state: %s)", state)
		}
	}

	fmt.Println()
	printSuccess("Server starting!")
	printInfo("Auth server:  localhost:3724")
	printInfo("World server: localhost:8085")
	printInfo("MySQL:        localhost:3306")
	fmt.Println()
	printInfo("View logs:       mithril server logs")
	printInfo("Attach console:  mithril server attach")
	printInfo("Stop server:     mithril server stop")
	return nil
}

func serverStop(cfg *Config) error {
	printInfo("Stopping Mithril TrinityCore server...")
	if err := dockerCompose(cfg, "down"); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}
	printSuccess("Server stopped")
	return nil
}

func serverRestart(cfg *Config) error {
	printInfo("Restarting Mithril TrinityCore server...")
	if err := dockerCompose(cfg, "restart"); err != nil {
		return fmt.Errorf("failed to restart server: %w", err)
	}
	printSuccess("Server restarted")
	return nil
}

func serverRebuild(cfg *Config) error {
	printInfo("Rebuilding TrinityCore inside the running container (incremental)...")

	containerID, err := composeContainerID(cfg)
	if err != nil || containerID == "" {
		return fmt.Errorf("server container is not running — start it with 'mithril server start'")
	}

	// Run cmake + make + make install inside the container.
	// The build directory and object files persist in the container,
	// so only changed source files are recompiled.
	rebuildScript := `set -e
cd /src/TrinityCore/build
echo "=== Running cmake ==="
cmake ../ \
    -DCMAKE_INSTALL_PREFIX=/opt/trinitycore \
    -DTOOLS=1 \
    -DWITH_WARNINGS=0 \
    -DCMAKE_C_COMPILER=clang \
    -DCMAKE_CXX_COMPILER=clang++ \
    -DUSE_SCRIPTPCH=0
echo "=== Compiling (incremental) ==="
make -j $(nproc)
echo "=== Installing ==="
make install
echo "=== Rebuild complete ==="
`

	cmd := exec.Command("docker", "exec", containerID, "bash", "-c", rebuildScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rebuild failed: %w", err)
	}

	printSuccess("TrinityCore rebuilt successfully")
	fmt.Println()
	printInfo("Restart the server to use the new build:")
	printInfo("  mithril server restart")
	return nil
}

func serverStatus(cfg *Config) error {
	return dockerCompose(cfg, "ps")
}

func serverLogs(cfg *Config) error {
	return dockerCompose(cfg, "logs", "-f")
}

func serverAttach(cfg *Config) error {
	printInfo("Attaching to worldserver console (Ctrl+P, Ctrl+Q to detach)...")

	containerID, err := composeContainerID(cfg)
	if err != nil {
		return fmt.Errorf("failed to find server container: %w", err)
	}
	if containerID == "" {
		return fmt.Errorf("server container is not running — start it with 'mithril server start'")
	}

	// Wait for the container to reach "running" state before attaching.
	// After "docker compose up -d", the container may still be starting or
	// restarting (e.g. while loading the world database), and docker attach
	// rejects containers that are not in a running state.
	const maxWait = 30
	for i := 0; i < maxWait; i++ {
		state, err := containerState(containerID)
		if err != nil {
			return fmt.Errorf("failed to inspect server container: %w", err)
		}
		if state == "running" {
			break
		}
		if i == 0 {
			printInfo(fmt.Sprintf("Container is %s, waiting for it to be running...", state))
		}
		if i == maxWait-1 {
			return fmt.Errorf("timed out waiting for container to be running (current state: %s)", state)
		}
		time.Sleep(1 * time.Second)
	}

	// Attach to the container
	attachCmd := exec.Command("docker", "attach", containerID)
	attachCmd.Stdin = os.Stdin
	attachCmd.Stdout = os.Stdout
	attachCmd.Stderr = os.Stderr
	return attachCmd.Run()
}

// composeContainerID returns the Docker container ID for the "server" service
// defined in the project's docker-compose.yml, or an empty string if no
// container exists.
func composeContainerID(cfg *Config) (string, error) {
	cmd := exec.Command("docker", "compose",
		"-p", cfg.DockerProjectName,
		"-f", cfg.DockerComposeFile,
		"ps", "-q", "server")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// containerState returns the current state of a Docker container (e.g.
// "running", "restarting", "exited") by inspecting it.
func containerState(containerID string) (string, error) {
	cmd := exec.Command("docker", "inspect",
		"--format", "{{.State.Status}}", containerID)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
