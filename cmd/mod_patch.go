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
	case "create":
		return runModPatchCreate(args)
	case "list":
		return runModPatchList(args)
	case "apply":
		return runModPatchApply(args)
	case "status":
		return runModPatchStatus(args)
	case "restore":
		return runModPatchRestore(args)
	case "remove":
		return runModPatchRemove(args)
	case "-h", "--help", "help":
		fmt.Print(patchUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod patch command: %s", subcmd)
	}
}

// runModPatchCreate scaffolds a new binary patch JSON file in a mod.
func runModPatchCreate(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod patch create <name> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	patchName := remaining[0]

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Sanitize name
	safeName := strings.ReplaceAll(strings.ToLower(patchName), " ", "-")
	if !strings.HasSuffix(safeName, ".json") {
		safeName += ".json"
	}

	patchDir := filepath.Join(cfg.ModDir(modName), "binary-patches")
	patchPath := filepath.Join(patchDir, safeName)

	if _, err := os.Stat(patchPath); err == nil {
		return fmt.Errorf("patch file already exists: %s", patchPath)
	}

	if err := os.MkdirAll(patchDir, 0755); err != nil {
		return fmt.Errorf("create binary-patches dir: %w", err)
	}

	template := fmt.Sprintf(`{
  "name": "%s",
  "description": "TODO: describe what this patch does",
  "patches": [
    { "address": "0x000000", "bytes": ["0x00"] }
  ]
}
`, strings.TrimSuffix(safeName, ".json"))

	if err := os.WriteFile(patchPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("write patch file: %w", err)
	}

	fmt.Printf("✓ Created binary patch: %s\n", patchPath)
	fmt.Printf("  Apply: mithril mod patch apply --mod %s\n", modName)
	return nil
}

// runModPatchRemove removes a binary patch JSON file from a mod.
// If patches from this mod are applied, prompts to restore Wow.exe and reset the tracker
// so other patches can be cleanly re-applied.
func runModPatchRemove(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod patch remove <name> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	patchName := remaining[0]
	if !strings.HasSuffix(patchName, ".json") {
		patchName += ".json"
	}

	patchPath := filepath.Join(cfg.ModDir(modName), "binary-patches", patchName)
	if _, err := os.Stat(patchPath); os.IsNotExist(err) {
		return fmt.Errorf("patch file not found: %s", patchPath)
	}

	// Check if this patch is currently applied
	trackerPath := filepath.Join(cfg.ModulesDir, "binary_patches_applied.json")
	tracker, _ := patcher.LoadTracker(trackerPath)
	trackerName := modName + "/binary-patches/" + patchName
	wasApplied := tracker.IsApplied(trackerName)

	if wasApplied {
		fmt.Printf("Binary patch '%s' is currently applied to Wow.exe.\n", patchName)
		if promptYesNo("Restore Wow.exe from clean backup and reset patch tracker?") {
			wowExePath := filepath.Join(cfg.ClientDir, "Wow.exe")
			if err := patcher.RestoreFromBackup(wowExePath); err != nil {
				fmt.Printf("  ⚠ Failed to restore backup: %v\n", err)
			} else {
				fmt.Println("  ✓ Restored Wow.exe from clean backup")
			}

			// Reset the tracker — remove all entries so other patches can be re-applied cleanly
			emptyTracker := &patcher.Tracker{}
			if err := patcher.SaveTracker(trackerPath, emptyTracker); err != nil {
				fmt.Printf("  ⚠ Failed to reset tracker: %v\n", err)
			} else {
				fmt.Println("  ✓ Patch tracker reset")
			}

			// Check if other patches need re-applying
			var otherPatches []string
			for _, ap := range tracker.Applied {
				if ap.Name != trackerName {
					otherPatches = append(otherPatches, ap.Name)
				}
			}
			if len(otherPatches) > 0 {
				fmt.Printf("\n  The following patches were also cleared and need to be re-applied:\n")
				for _, name := range otherPatches {
					fmt.Printf("    - %s\n", name)
				}
				fmt.Println("  Run 'mithril mod patch apply ...' to re-apply them.")
			}
		} else {
			fmt.Println("  Skipping restore — Wow.exe retains the applied patch bytes.")
		}
	}

	// Remove the patch file
	if err := os.Remove(patchPath); err != nil {
		return fmt.Errorf("remove patch file: %w", err)
	}

	// Clean up empty binary-patches/ directory
	cleanEmptyDirs(filepath.Join(cfg.ModDir(modName), "binary-patches"))

	fmt.Printf("✓ Removed binary patch: %s\n", patchName)
	return nil
}

const patchUsage = `Mithril Mod Patch - Binary patches for Wow.exe

Usage:
  mithril mod patch <command> [args]

Commands:
  create <name> --mod <name>
                            Scaffold a binary patch JSON file in a mod
  remove <name> --mod <name>
                            Remove a binary patch JSON file from a mod
  list                      List available patches from installed mods
  apply --mod <name>        Apply all patches from a mod's binary-patches/ directory
  apply <path> [...]        Apply one or more specific patch JSON files
  status                    Show which patches have been applied
  restore                   Restore Wow.exe from clean backup

Examples:
  mithril mod patch create my-fix --mod my-mod
  mithril mod patch apply --mod my-mod
  mithril mod patch remove my-fix --mod my-mod
  mithril mod patch list
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

	// Copy DLL files from the mod's binary-patches/ directory
	dllsCopied := 0
	if modName != "" {
		dc, err := deployModDLLs(cfg, modName, tracker, trackerPath)
		if err != nil {
			fmt.Printf("  ⚠ DLL deploy error: %v\n", err)
		}
		dllsCopied += dc
	}

	if applied > 0 || dllsCopied > 0 {
		fmt.Printf("\nApplied %d new patch(es)", applied)
		if dllsCopied > 0 {
			fmt.Printf(", copied %d DLL(s)", dllsCopied)
		}
		fmt.Println(" to client")
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
	if err := restoreWowExe(cfg); err != nil {
		return err
	}
	fmt.Println("✓ Restored Wow.exe from clean backup")
	fmt.Println("  All binary patches have been cleared")
	fmt.Println("  Run 'mithril mod patch apply ...' to reapply patches")
	return nil
}

// restoreWowExe restores Wow.exe from the clean backup and clears the patch tracker.
func restoreWowExe(cfg *Config) error {
	wowExePath := filepath.Join(cfg.ClientDir, "Wow.exe")
	if err := patcher.RestoreFromBackup(wowExePath); err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	// Clear the tracker
	trackerPath := filepath.Join(cfg.ModulesDir, "binary_patches_applied.json")
	tracker := &patcher.Tracker{}
	if err := patcher.SaveTracker(trackerPath, tracker); err != nil {
		return fmt.Errorf("clear tracker: %w", err)
	}

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

// deployModDLLs copies any .dll files from a mod's binary-patches/ directory
// to the client directory (next to Wow.exe) and tracks them in the binary patch tracker.
func deployModDLLs(cfg *Config, modName string, tracker *patcher.Tracker, trackerPath string) (int, error) {
	patchDir := filepath.Join(cfg.ModDir(modName), "binary-patches")
	entries, err := os.ReadDir(patchDir)
	if err != nil {
		return 0, nil // no binary-patches dir
	}

	copied := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".dll") {
			continue
		}

		trackerName := modName + "/binary-patches/" + name

		// Check if already deployed via checksum
		srcPath := filepath.Join(patchDir, name)
		dstPath := filepath.Join(cfg.ClientDir, name)
		srcHash := fileChecksum(srcPath)
		dstHash := fileChecksum(dstPath)

		if tracker.IsApplied(trackerName) && srcHash == dstHash {
			continue // already up to date
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return copied, fmt.Errorf("copy %s: %w", name, err)
		}
		fmt.Printf("  ✓ %s → %s\n", name, cfg.ClientDir)

		if !tracker.IsApplied(trackerName) {
			tracker.MarkApplied(trackerName, timeNow())
		}
		copied++
	}

	if copied > 0 {
		if err := patcher.SaveTracker(trackerPath, tracker); err != nil {
			return copied, fmt.Errorf("save tracker: %w", err)
		}
	}

	return copied, nil
}

// findBinaryPatches returns the filenames of all .json and .dll files in a mod's binary-patches/ directory.
func findBinaryPatches(cfg *Config, modName string) []string {
	patchDir := filepath.Join(cfg.ModDir(modName), "binary-patches")
	entries, err := os.ReadDir(patchDir)
	if err != nil {
		return nil
	}
	var patches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		patches = append(patches, e.Name())
	}
	return patches
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
