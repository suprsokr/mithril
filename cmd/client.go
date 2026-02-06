package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// promptAndCopyClient asks the user where their WoW 3.3.5a client lives,
// validates it, and copies the whole directory into mithril-data/client/ so
// we have a dedicated copy for extraction and future client-side modding.
func promptAndCopyClient(cfg *Config) error {
	// Already have a copy? Skip.
	if fileExists(filepath.Join(cfg.ClientDir, "Data")) {
		printSuccess("Client already present at " + cfg.ClientDir)
		return nil
	}

	fmt.Println("  Mithril needs a copy of your WoW 3.3.5a client.")
	fmt.Println("  This copy is used to extract server data (maps, vmaps, mmaps)")
	fmt.Println("  and will also serve as the working client for any mods you apply.")
	fmt.Println()

	clientPath, err := promptClientPath()
	if err != nil {
		return err
	}

	fmt.Println()
	printWarning("Mithril will copy the client to:")
	printInfo("  " + cfg.ClientDir)
	fmt.Println()
	printInfo("Your original client will not be modified.")
	printInfo("The copy may use several GB of disk space.")
	fmt.Println()

	if !promptYesNo("Proceed with copy?") {
		return fmt.Errorf("cancelled by user")
	}

	printInfo("Copying client files (this may take a few minutes)...")
	if err := copyDir(clientPath, cfg.ClientDir); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}
	printSuccess("Client copied to " + cfg.ClientDir)
	return nil
}

// promptClientPath interactively asks for the client directory and validates
// that it contains a Data/ folder with MPQ files.
func promptClientPath() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("  Enter the path to your WoW 3.3.5a client directory: ")
		if !scanner.Scan() {
			return "", fmt.Errorf("no input received")
		}
		raw := strings.TrimSpace(scanner.Text())

		// Expand ~ home shorthand.
		if strings.HasPrefix(raw, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				raw = filepath.Join(home, raw[2:])
			}
		}
		abs, err := filepath.Abs(raw)
		if err == nil {
			raw = abs
		}

		if !fileExists(filepath.Join(raw, "Data")) {
			printWarning("No 'Data' directory found at: " + raw)
			printInfo("The WoW client directory should contain a 'Data' folder with .MPQ files.")
			fmt.Println()
			continue
		}
		return raw, nil
	}
}

// runClient dispatches client subcommands.
func runClient(subcmd string, _ []string) error {
	cfg := DefaultConfig()
	switch subcmd {
	case "start":
		return clientStart(cfg)
	default:
		return fmt.Errorf("unknown client subcommand: %s (use start)", subcmd)
	}
}

// clientStart launches the WoW 3.3.5a client executable.
// On Windows the .exe is started directly; on Linux and macOS it is launched
// through Wine.
func clientStart(cfg *Config) error {
	wowExe := filepath.Join(cfg.ClientDir, "Wow.exe")
	if !fileExists(wowExe) {
		return fmt.Errorf("client not found at %s\nRun 'mithril init' first to copy the WoW client", wowExe)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command(wowExe)
	default:
		// Check that Wine is available.
		if _, err := exec.LookPath("wine"); err != nil {
			printWarning("Wine is required to run Wow.exe on " + runtime.GOOS + ".")
			printInfo("Install Wine and try again:")
			printInfo("  Ubuntu/Debian: sudo apt install wine")
			printInfo("  macOS:         brew install --cask wine-stable")
			return fmt.Errorf("wine not found: %w", err)
		}
		cmd = exec.Command("wine", wowExe)
	}

	cmd.Dir = cfg.ClientDir
	// Detach stdout/stderr so Wine logs don't flood the terminal.
	cmd.Stdout = nil
	cmd.Stderr = nil

	printInfo("Launching WoW client...")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch client: %w", err)
	}

	// Release the child process so it keeps running after mithril exits.
	if err := cmd.Process.Release(); err != nil {
		printWarning("Could not fully detach client process: " + err.Error())
	}

	printSuccess("WoW client launched (PID " + fmt.Sprintf("%d", cmd.Process.Pid) + ")")
	return nil
}

// extractClientData runs the map/vmap/mmap extractors inside a disposable
// Docker container, mounting the local client and data directories.
func extractClientData(cfg *Config) error {
	if fileExists(filepath.Join(cfg.DataDir, "maps")) &&
		fileExists(filepath.Join(cfg.DataDir, "vmaps")) {
		printInfo("Extracted data already exists, skipping extraction.")
		return nil
	}

	printInfo("Running map extractors inside Docker container...")
	printInfo("This can take 30+ minutes depending on your hardware.")
	fmt.Println()

	return runCmd("docker", "run", "--rm",
		"-v", cfg.ClientDir+":/opt/trinitycore/client",
		"-v", cfg.DataDir+":/opt/trinitycore/data",
		"mithril-server:latest",
		"/usr/local/bin/extract-data.sh", "/opt/trinitycore/client",
	)
}
