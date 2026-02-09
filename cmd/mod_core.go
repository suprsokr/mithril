package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func runModCore(subcmd string, args []string) error {
	switch subcmd {
	case "create":
		return runModCoreCreate(args)
	case "list":
		return runModCoreList(args)
	case "apply":
		return runModCoreApply(args)
	case "status":
		return runModCoreStatus(args)
	case "remove":
		return runModCoreRemove(args)
	case "-h", "--help", "help":
		fmt.Print(coreUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod core command: %s", subcmd)
	}
}

// runModCoreCreate scaffolds a new core patch file in a mod.
func runModCoreCreate(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod core create <name> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	patchName := remaining[0]

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Sanitize name
	safeName := strings.ReplaceAll(strings.ToLower(patchName), " ", "-")
	if !strings.HasSuffix(safeName, ".patch") {
		safeName += ".patch"
	}

	patchDir := filepath.Join(cfg.ModDir(modName), "core-patches")
	patchPath := filepath.Join(patchDir, safeName)

	if _, err := os.Stat(patchPath); err == nil {
		return fmt.Errorf("core patch file already exists: %s", patchPath)
	}

	if err := os.MkdirAll(patchDir, 0755); err != nil {
		return fmt.Errorf("create core-patches dir: %w", err)
	}

	template := fmt.Sprintf(`# Core patch: %s
# Mod: %s
#
# To generate from a TrinityCore fork:
#   git diff > %s
# or for committed changes:
#   git format-patch -1 HEAD --stdout > %s
#
# Place this file in: modules/%s/core-patches/
`, patchName, modName, safeName, safeName, modName)

	if err := os.WriteFile(patchPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("write core patch file: %w", err)
	}

	fmt.Printf("✓ Created core patch: %s\n", patchPath)
	fmt.Printf("  Apply: mithril mod core apply --mod %s\n", modName)
	return nil
}

// runModCoreRemove removes a core patch file from a mod.
func runModCoreRemove(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod core remove <name> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	patchName := remaining[0]
	if !strings.HasSuffix(patchName, ".patch") && !strings.HasSuffix(patchName, ".diff") {
		patchName += ".patch"
	}

	patchPath := filepath.Join(cfg.ModDir(modName), "core-patches", patchName)
	if _, err := os.Stat(patchPath); os.IsNotExist(err) {
		return fmt.Errorf("core patch file not found: %s", patchPath)
	}

	if err := os.Remove(patchPath); err != nil {
		return fmt.Errorf("remove core patch file: %w", err)
	}

	// Clean up empty core-patches/ directory
	cleanEmptyDirs(filepath.Join(cfg.ModDir(modName), "core-patches"))

	fmt.Printf("✓ Removed core patch: %s\n", patchName)
	return nil
}

const coreUsage = `Mithril Mod Core - TrinityCore server core patches

Usage:
  mithril mod core <command> [args]

Commands:
  create <name> --mod <mod> Scaffold a core patch file
  remove <name> --mod <mod> Remove a core patch file
  list [--mod <mod>]        List core patches and their status
  apply [--mod <mod>]       Apply pending core patches to TrinityCore
  status [--mod <mod>]      Show which core patches are applied

Core patches are standard git .patch files placed in a mod's core-patches/ directory.
After applying, you must rebuild the server:
  mithril mod core apply --mod my-mod
  mithril init --rebuild

Creating patches manually:
  Make changes in a TrinityCore fork, then:
    git diff > my-change.patch
  or for committed changes:
    git format-patch -1 HEAD --stdout > my-change.patch
  Place the .patch file in: modules/<mod>/core-patches/

Examples:
  mithril mod core create enable-feature --mod my-mod
  mithril mod core remove enable-feature --mod my-mod
  mithril mod core list
  mithril mod core apply --mod my-mod
  mithril mod core status
`

// CorePatchTracker records which core patches have been applied.
type CorePatchTracker struct {
	Applied []AppliedCorePatch `json:"applied"`
}

// AppliedCorePatch tracks a single core patch.
type AppliedCorePatch struct {
	Mod       string `json:"mod"`
	File      string `json:"file"`
	AppliedAt string `json:"applied_at"`
}

func (t *CorePatchTracker) IsApplied(mod, file string) bool {
	for _, a := range t.Applied {
		if a.Mod == mod && a.File == file {
			return true
		}
	}
	return false
}

func loadCoreTracker(cfg *Config) (*CorePatchTracker, error) {
	path := filepath.Join(cfg.ModulesDir, "core_patches_applied.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CorePatchTracker{}, nil
		}
		return nil, err
	}
	var t CorePatchTracker
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func saveCoreTracker(cfg *Config, t *CorePatchTracker) error {
	path := filepath.Join(cfg.ModulesDir, "core_patches_applied.json")
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// corePatchInfo describes a core patch file.
type corePatchInfo struct {
	mod      string
	filename string
	path     string
}

// findCorePatches discovers .patch files in a mod's core-patches/ directory.
func findCorePatches(cfg *Config, modName string) []corePatchInfo {
	patchDir := filepath.Join(cfg.ModDir(modName), "core-patches")
	if _, err := os.Stat(patchDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(patchDir)
	if err != nil {
		return nil
	}

	var patches []corePatchInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".patch") || strings.HasSuffix(entry.Name(), ".diff") {
			patches = append(patches, corePatchInfo{
				mod:      modName,
				filename: entry.Name(),
				path:     filepath.Join(patchDir, entry.Name()),
			})
		}
	}

	sort.Slice(patches, func(i, j int) bool {
		return patches[i].filename < patches[j].filename
	})

	return patches
}

func runModCoreList(args []string) error {
	modName, _ := parseModFlag(args)
	cfg := DefaultConfig()
	tracker, _ := loadCoreTracker(cfg)

	var mods []string
	if modName != "" {
		mods = []string{modName}
	} else {
		mods = getAllMods(cfg)
	}

	totalPatches := 0
	for _, mod := range mods {
		patches := findCorePatches(cfg, mod)
		if len(patches) == 0 {
			continue
		}

		fmt.Printf("Mod '%s':\n", mod)
		for _, p := range patches {
			status := "pending"
			if tracker.IsApplied(p.mod, p.filename) {
				status = "✓ applied"
			}
			fmt.Printf("  [%-10s] %s\n", status, p.filename)
		}
		totalPatches += len(patches)
		fmt.Println()
	}

	if totalPatches == 0 {
		fmt.Println("No core patches found.")
		fmt.Println("Place .patch files in: modules/<mod>/core-patches/")
	}

	return nil
}

func runModCoreStatus(args []string) error {
	return runModCoreList(args)
}

func runModCoreApply(args []string) error {
	modName, _ := parseModFlag(args)
	cfg := DefaultConfig()
	tracker, err := loadCoreTracker(cfg)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}

	var mods []string
	if modName != "" {
		mods = []string{modName}
	} else {
		mods = getAllMods(cfg)
	}

	// Check that the TrinityCore source exists
	tcSourceDir := cfg.SourceDir
	if _, err := os.Stat(tcSourceDir); os.IsNotExist(err) {
		return fmt.Errorf("TrinityCore source not found at %s\nRun 'mithril init' first to clone the source", tcSourceDir)
	}

	// Check that it's a git repo
	gitDir := filepath.Join(tcSourceDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("TrinityCore source at %s is not a git repository", tcSourceDir)
	}

	applied := 0
	for _, mod := range mods {
		patches := findCorePatches(cfg, mod)
		if len(patches) == 0 {
			continue
		}

		for _, p := range patches {
			if tracker.IsApplied(p.mod, p.filename) {
				continue
			}

			fmt.Printf("Applying %s/%s...\n", p.mod, p.filename)

			// First, check if the patch applies cleanly
			checkCmd := exec.Command("git", "apply", "--check", p.path)
			checkCmd.Dir = tcSourceDir
			if checkOutput, err := checkCmd.CombinedOutput(); err != nil {
				fmt.Printf("  ⚠ Patch does not apply cleanly: %s\n", strings.TrimSpace(string(checkOutput)))

				// Try with 3-way merge
				fmt.Println("  Trying with 3-way merge...")
				checkCmd3 := exec.Command("git", "apply", "--check", "--3way", p.path)
				checkCmd3.Dir = tcSourceDir
				if checkOutput3, err := checkCmd3.CombinedOutput(); err != nil {
					fmt.Printf("  ⚠ Patch cannot be applied: %s\n", strings.TrimSpace(string(checkOutput3)))
					return fmt.Errorf("patch %s failed — stopping to prevent partial application", p.filename)
				}
			}

			// Apply the patch
			applyCmd := exec.Command("git", "apply", "--stat", p.path)
			applyCmd.Dir = tcSourceDir
			if statOutput, err := applyCmd.CombinedOutput(); err == nil {
				fmt.Printf("  %s", string(statOutput))
			}

			applyCmd2 := exec.Command("git", "apply", p.path)
			applyCmd2.Dir = tcSourceDir
			if output, err := applyCmd2.CombinedOutput(); err != nil {
				fmt.Printf("  ⚠ Failed to apply: %s\n", strings.TrimSpace(string(output)))
				return fmt.Errorf("patch %s failed — stopping to prevent partial application", p.filename)
			}

			tracker.Applied = append(tracker.Applied, AppliedCorePatch{
				Mod:       p.mod,
				File:      p.filename,
				AppliedAt: timeNow(),
			})

			fmt.Printf("  ✓ %s\n", p.filename)
			applied++
		}
	}

	// Save tracker
	if err := saveCoreTracker(cfg, tracker); err != nil {
		return fmt.Errorf("save tracker: %w", err)
	}

	if applied == 0 {
		fmt.Println("No pending core patches to apply.")
	} else {
		fmt.Printf("\n✓ Applied %d core patch(es) to TrinityCore\n", applied)
		fmt.Println()
		fmt.Println("To use the patched server, rebuild the Docker image:")
		fmt.Println("  mithril init --rebuild")
		fmt.Println()
		fmt.Println("Note: This will recompile TrinityCore with your changes.")
	}

	return nil
}
