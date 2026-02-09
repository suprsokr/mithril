package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

const cleanUsage = `Mithril Clean - Remove the mithril-data workspace and Docker resources

Usage:
  mithril clean [flags]

Flags:
  --all             Remove everything including mods (default: mods are preserved)
  -h, --help        Show this help message

By default, mods are backed up before cleaning and restored afterward.
Use --all to remove everything with no backup.

Examples:
  mithril clean               # Clean workspace, preserve mods
  mithril clean --all         # Clean everything including mods
`

func runClean(args []string) error {
	// Parse flags
	keepMods := true
	for _, arg := range args {
		switch arg {
		case "--all":
			keepMods = false
		case "-h", "--help", "help":
			fmt.Print(cleanUsage)
			return nil
		default:
			return fmt.Errorf("unknown flag: %s\n\n%s", arg, cleanUsage)
		}
	}

	cfg := DefaultConfig()

	if !fileExists(cfg.MithrilDir) {
		fmt.Println("Nothing to clean — mithril-data/ does not exist.")
		return nil
	}

	// Confirm
	if keepMods {
		fmt.Println("This will remove mithril-data/ and Docker resources, but preserve your mods.")
	} else {
		fmt.Println("This will remove ALL of mithril-data/ and Docker resources, including all mods.")
	}
	if !promptYesNo("Continue?") {
		fmt.Println("Aborted.")
		return nil
	}

	// Back up mods if requested
	var backupDir string
	if keepMods {
		mods := findModDirs(cfg)
		if len(mods) > 0 {
			var err error
			backupDir, err = backupMods(cfg, mods)
			if err != nil {
				return fmt.Errorf("failed to backup mods: %w", err)
			}
			fmt.Printf("  ✓ Backed up %d mod(s) to %s\n", len(mods), backupDir)
		}
	}

	// Stop and remove Docker containers + volumes
	if fileExists(cfg.DockerComposeFile) {
		fmt.Println("Stopping Docker containers...")
		dockerCompose(cfg, "down", "-v") // best-effort, ignore errors
		fmt.Println("  ✓ Docker containers and volumes removed")
	}

	// Remove Docker image
	fmt.Println("Removing Docker image...")
	runCmd("docker", "rmi", "mithril-server:latest") // best-effort
	fmt.Println("  ✓ Docker image removed")

	// Remove mithril-data/
	fmt.Println("Removing mithril-data/...")
	if err := os.RemoveAll(cfg.MithrilDir); err != nil {
		return fmt.Errorf("failed to remove mithril-data: %w", err)
	}
	fmt.Println("  ✓ mithril-data/ removed")

	// Restore mods
	if keepMods && backupDir != "" {
		fmt.Println("Restoring mods...")
		if err := restoreMods(cfg, backupDir); err != nil {
			return fmt.Errorf("failed to restore mods (backup at %s): %w", backupDir, err)
		}
		// Clean up temp backup
		os.RemoveAll(backupDir)
		fmt.Println("  ✓ Mods restored")
	}

	fmt.Println()
	printSuccess("Clean complete!")
	if keepMods {
		fmt.Println("  Your mods have been preserved in mithril-data/modules/")
	}
	fmt.Println("  Run 'mithril init' to set up a fresh environment.")
	return nil
}

// findModDirs returns the names of all mod directories (excludes baseline, build, and tracker files).
func findModDirs(cfg *Config) []string {
	entries, err := os.ReadDir(cfg.ModulesDir)
	if err != nil {
		return nil
	}

	skip := map[string]bool{
		"baseline": true,
		"build":    true,
	}

	var mods []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if skip[e.Name()] {
			continue
		}
		// Must have a mod.json to be considered a mod
		if fileExists(filepath.Join(cfg.ModDir(e.Name()), "mod.json")) {
			mods = append(mods, e.Name())
		}
	}
	return mods
}

// backupMods copies mod directories and the manifest to a temp directory.
func backupMods(cfg *Config, mods []string) (string, error) {
	backupDir, err := os.MkdirTemp("", "mithril-mod-backup-*")
	if err != nil {
		return "", err
	}

	for _, mod := range mods {
		src := cfg.ModDir(mod)
		dst := filepath.Join(backupDir, mod)
		if err := copyDir(src, dst); err != nil {
			return backupDir, fmt.Errorf("backup mod %s: %w", mod, err)
		}
	}

	// Also back up manifest.json if it exists (preserves build_order)
	manifestSrc := filepath.Join(cfg.ModulesDir, "manifest.json")
	if fileExists(manifestSrc) {
		data, err := os.ReadFile(manifestSrc)
		if err == nil {
			os.WriteFile(filepath.Join(backupDir, "manifest.json"), data, 0644)
		}
	}

	return backupDir, nil
}

// restoreMods copies backed-up mods back into the modules directory.
func restoreMods(cfg *Config, backupDir string) error {
	// Ensure modules dir exists
	if err := os.MkdirAll(cfg.ModulesDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		src := filepath.Join(backupDir, e.Name())
		dst := filepath.Join(cfg.ModulesDir, e.Name())
		if e.IsDir() {
			if err := copyDir(src, dst); err != nil {
				return fmt.Errorf("restore mod %s: %w", e.Name(), err)
			}
		} else {
			// manifest.json
			data, err := os.ReadFile(src)
			if err == nil {
				os.WriteFile(dst, data, 0644)
			}
		}
	}
	return nil
}

