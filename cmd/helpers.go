package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Shell / Docker helpers
// ---------------------------------------------------------------------------

// runCmd executes a command, streaming stdout/stderr to the terminal.
func runCmd(name string, args ...string) error {
	fmt.Printf("› %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// runCmdDir executes a command in a specific working directory.
func runCmdDir(dir, name string, args ...string) error {
	fmt.Printf("› (in %s) %s %s\n", dir, name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// dockerCompose runs `docker compose` with the project name and compose file.
func dockerCompose(cfg *Config, args ...string) error {
	base := []string{
		"compose",
		"-p", cfg.DockerProjectName,
		"-f", cfg.DockerComposeFile,
	}
	return runCmd("docker", append(base, args...)...)
}

// dockerComposeExec runs `docker compose exec <service> <cmd>`.
func dockerComposeExec(cfg *Config, service string, cmdArgs ...string) error {
	args := append([]string{"exec", service}, cmdArgs...)
	return dockerCompose(cfg, args...)
}

// ---------------------------------------------------------------------------
// File-system helpers
// ---------------------------------------------------------------------------

// fileExists returns true if the path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
		} else {
			if err := copyFile(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copies a single file preserving its permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// downloadFile downloads url to the given path, showing a progress indicator.
func downloadFile(dst string, url string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	progress := &writeCounter{Total: resp.ContentLength}
	_, err = io.Copy(out, io.TeeReader(resp.Body, progress))
	fmt.Println() // newline after progress bar
	return err
}

type writeCounter struct {
	Total      int64
	Downloaded int64
}

func (wc *writeCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Downloaded += int64(n)
	if wc.Total > 0 {
		pct := float64(wc.Downloaded) / float64(wc.Total) * 100
		fmt.Fprintf(os.Stderr, "\r  Downloading... %.1f%% (%d / %d MB)",
			pct, wc.Downloaded>>20, wc.Total>>20)
	} else {
		fmt.Fprintf(os.Stderr, "\r  Downloading... %d MB", wc.Downloaded>>20)
	}
	return n, nil
}

// cleanEmptyDirs removes empty directories bottom-up starting from the given root.
func cleanEmptyDirs(root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		return nil // just walk to collect paths
	})
	// Walk bottom-up by collecting dirs first
	var dirs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	// Remove in reverse order (deepest first)
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err == nil && len(entries) == 0 {
			os.Remove(dirs[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Interactive helpers
// ---------------------------------------------------------------------------

// promptYesNo asks a yes/no question and returns true for yes (default).
func promptYesNo(question string) bool {
	fmt.Printf("%s [Y/n] ", question)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "" || answer == "y" || answer == "yes"
	}
	return true
}

// ---------------------------------------------------------------------------
// Pretty printing
// ---------------------------------------------------------------------------

func printStep(step, total int, msg string) {
	fmt.Printf("\n\033[1;36m[%d/%d]\033[0m \033[1m%s\033[0m\n", step, total, msg)
}

func printSuccess(msg string) {
	fmt.Printf("\033[1;32m✓\033[0m %s\n", msg)
}

func printWarning(msg string) {
	fmt.Printf("\033[1;33m⚠\033[0m %s\n", msg)
}

func printInfo(msg string) {
	fmt.Printf("\033[1;34mℹ\033[0m %s\n", msg)
}

