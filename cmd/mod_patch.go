package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/suprsokr/mithril/internal/patcher"
)

func runModPatch(subcmd string, args []string) error {
	switch subcmd {
	case "list":
		return runModPatchList(args)
	case "apply":
		return runModPatchApply(args)
	case "status":
		return runModPatchStatus(args)
	case "restore":
		return runModPatchRestore(args)
	case "-h", "--help", "help":
		fmt.Print(patchUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod patch command: %s", subcmd)
	}
}

const patchUsage = `Mithril Mod Patch - Binary patches for Wow.exe

Usage:
  mithril mod patch <command> [args]

Commands:
  list                      List available patches from installed mods
  apply --mod <name>        Apply all patches from a mod's binary-patches/ directory
  apply <path> [...]        Apply one or more specific patch JSON files
  status                    Show which patches have been applied
  restore                   Restore Wow.exe from clean backup

Patches are distributed as mods with JSON files in their binary-patches/ directories.
Use 'mithril mod patch list' to see all available patches from your installed mods.

Examples:
  mithril mod patch list
  mithril mod patch apply --mod allow-custom-gluexml
  mithril mod patch apply my-mod/binary-patches/my-patch.json
  mithril mod patch status
  mithril mod patch restore
`

func runModPatchList(args []string) error {
	cfg := DefaultConfig()

	fmt.Println("=== Available Binary Patches ===")
	fmt.Println()

	// Per-mod patches
	mods := getAllMods(cfg)
	found := false
	for _, mod := range mods {
		patchDir := filepath.Join(cfg.ModDir(mod), "binary-patches")
		entries, err := os.ReadDir(patchDir)
		if err != nil {
			continue
		}
		first := true
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if first {
				fmt.Printf("Mod '%s':\n", mod)
				first = false
				found = true
			}
			pf, err := patcher.LoadPatchFile(filepath.Join(patchDir, entry.Name()))
			desc := ""
			if err == nil && pf.Description != "" {
				desc = pf.Description
			}
			applyPath := mod + "/binary-patches/" + entry.Name()
			fmt.Printf("  %-50s %s\n", applyPath, desc)
		}
		if !first {
			fmt.Println()
		}
	}

	if !found {
		fmt.Println("No patches found. Binary patches are distributed as mods.")
		fmt.Println("Install a mod that includes a binary-patches/ directory to see patches here.")
	}

	return nil
}

func runModPatchApply(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mithril mod patch apply --mod <name> | <path> [...]")
	}

	cfg := DefaultConfig()

	// If --mod is specified, expand to all JSON files in that mod's binary-patches/ dir
	modName, remaining := parseModFlag(args)
	if modName != "" {
		patchDir := filepath.Join(cfg.ModDir(modName), "binary-patches")
		entries, err := os.ReadDir(patchDir)
		if err != nil {
			return fmt.Errorf("no binary-patches/ directory found in mod %s", modName)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				remaining = append(remaining, modName+"/binary-patches/"+entry.Name())
			}
		}
		if len(remaining) == 0 {
			return fmt.Errorf("no .json patch files found in %s", patchDir)
		}
		args = remaining
	}

	wowExePath := filepath.Join(cfg.ClientDir, "Wow.exe")

	if _, err := os.Stat(wowExePath); os.IsNotExist(err) {
		return fmt.Errorf("Wow.exe not found at %s", wowExePath)
	}

	// Ensure backup
	backupPath, err := patcher.EnsureBackup(wowExePath)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	fmt.Printf("Backup: %s\n", backupPath)

	// Verify clean backup
	isClean, actualMD5, err := patcher.VerifyCleanClient(backupPath)
	if err != nil {
		fmt.Printf("  ⚠ Could not verify backup: %v\n", err)
	} else if isClean {
		fmt.Println("  ✓ Clean WoW 3.3.5a client verified")
	} else {
		fmt.Printf("  ⚠ Backup MD5 %s does not match clean client (%s)\n", actualMD5, patcher.CleanClientMD5)
		fmt.Println("    Patches are designed for the clean 3.3.5a (12340) client")
	}

	// Load tracker
	trackerPath := filepath.Join(cfg.ModulesDir, "binary_patches_applied.json")
	tracker, _ := patcher.LoadTracker(trackerPath)

	// Always start from clean backup to ensure consistent state
	fmt.Println("\nRestoring from clean backup before applying patches...")
	if err := patcher.RestoreFromBackup(wowExePath); err != nil {
		return fmt.Errorf("restore from backup: %w", err)
	}

	// Collect all patches to apply (both already-tracked and new)
	type patchEntry struct {
		name     string
		pf       *patcher.PatchFile
	}

	// First, re-apply all previously tracked patches
	var allPatches []patchEntry
	for _, ap := range tracker.Applied {
		pf := resolvePatch(cfg, ap.Name)
		if pf != nil {
			allPatches = append(allPatches, patchEntry{name: ap.Name, pf: pf})
		}
	}

	// Then add new patches requested by the user
	applied := 0
	for _, arg := range args {
		name, pf, err := resolveUserPatch(cfg, arg)
		if err != nil {
			fmt.Printf("  ⚠ %v\n", err)
			continue
		}

		if tracker.IsApplied(name) {
			fmt.Printf("  Already applied: %s\n", name)
			continue
		}

		allPatches = append(allPatches, patchEntry{name: name, pf: pf})
		tracker.MarkApplied(name, timeNow())
		applied++
	}

	// Apply all patches in order
	for _, pe := range allPatches {
		if err := patcher.ApplyPatchFile(wowExePath, pe.pf); err != nil {
			fmt.Printf("  ⚠ Failed to apply %s: %v\n", pe.name, err)
			continue
		}
		fmt.Printf("  ✓ %s\n", pe.name)
	}

	// Save tracker
	if err := patcher.SaveTracker(trackerPath, tracker); err != nil {
		fmt.Printf("  ⚠ Could not save tracker: %v\n", err)
	}

	if applied > 0 {
		fmt.Printf("\nApplied %d new patch(es) to Wow.exe\n", applied)
	} else {
		fmt.Println("\nNo new patches to apply")
	}

	return nil
}

func runModPatchStatus(args []string) error {
	cfg := DefaultConfig()

	trackerPath := filepath.Join(cfg.ModulesDir, "binary_patches_applied.json")
	tracker, err := patcher.LoadTracker(trackerPath)
	if err != nil || len(tracker.Applied) == 0 {
		fmt.Println("No binary patches have been applied.")
		fmt.Println("Run 'mithril mod patch list' to see available patches.")
		return nil
	}

	fmt.Println("=== Applied Binary Patches ===")
	fmt.Println()
	for _, ap := range tracker.Applied {
		fmt.Printf("  ✓ %-35s (applied %s)\n", ap.Name, ap.AppliedAt)
	}

	// Check Wow.exe exists
	wowExePath := filepath.Join(cfg.ClientDir, "Wow.exe")
	if info, err := os.Stat(wowExePath); err == nil {
		fmt.Printf("\nWow.exe: %d bytes\n", info.Size())
	}

	backupPath := wowExePath + ".clean"
	if _, err := os.Stat(backupPath); err == nil {
		fmt.Println("Backup:  Wow.exe.clean (present)")
	}

	return nil
}

func runModPatchRestore(args []string) error {
	cfg := DefaultConfig()

	wowExePath := filepath.Join(cfg.ClientDir, "Wow.exe")
	if err := patcher.RestoreFromBackup(wowExePath); err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	// Clear the tracker
	trackerPath := filepath.Join(cfg.ModulesDir, "binary_patches_applied.json")
	tracker := &patcher.Tracker{}
	if err := patcher.SaveTracker(trackerPath, tracker); err != nil {
		fmt.Printf("  ⚠ Could not clear tracker: %v\n", err)
	}

	fmt.Println("✓ Restored Wow.exe from clean backup")
	fmt.Println("  All binary patches have been cleared")
	fmt.Println("  Run 'mithril mod patch apply ...' to reapply patches")

	return nil
}

// resolvePatch finds a patch by name (used for re-applying tracked patches).
// Name format: "modname/binary-patches/filename.json"
func resolvePatch(cfg *Config, name string) *patcher.PatchFile {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 {
		path := filepath.Join(cfg.ModDir(parts[0]), parts[1])
		pf, err := patcher.LoadPatchFile(path)
		if err == nil {
			return pf
		}
	}
	return nil
}

// resolveUserPatch resolves a user-provided patch argument to a name and PatchFile.
func resolveUserPatch(cfg *Config, arg string) (string, *patcher.PatchFile, error) {
	// Check if it's a file path (relative to modules dir)
	if strings.HasSuffix(arg, ".json") {
		// Try as a relative path from modules dir first
		modPath := filepath.Join(cfg.ModulesDir, arg)
		if pf, err := patcher.LoadPatchFile(modPath); err == nil {
			return filepath.ToSlash(arg), pf, nil
		}

		// Try as an absolute or workspace-relative path
		pf, err := patcher.LoadPatchFile(arg)
		if err != nil {
			return "", nil, fmt.Errorf("load patch file %s: %w", arg, err)
		}
		name := arg
		// If it's inside a mod, use relative path as the name
		rel, relErr := filepath.Rel(cfg.ModulesDir, arg)
		if relErr == nil {
			name = filepath.ToSlash(rel)
		}
		return name, pf, nil
	}

	return "", nil, fmt.Errorf("unknown patch: %s (use a .json file path, e.g., %s/binary-patches/%s.json)", arg, arg, arg)
}
