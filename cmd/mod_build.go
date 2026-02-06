package cmd

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/suprsokr/go-mpq"
	"github.com/suprsokr/mithril/internal/dbc"
)

// builtFile tracks a DBC that was converted from CSV and is ready to package.
type builtFile struct {
	diskPath string // path to the .dbc binary on disk
	mpqPath  string // path inside the MPQ (e.g., "DBFilesClient\Spell.dbc")
}

func runModBuild(args []string) error {
	modNames, _ := parseModFlags(args)
	cfg := DefaultConfig()

	// Ensure baseline exists
	if _, err := os.Stat(cfg.BaselineCsvDir); os.IsNotExist(err) {
		return fmt.Errorf("baseline not found — run 'mithril mod init' first")
	}

	fmt.Println("=== Mithril Mod Build ===")

	// Determine which mods to build
	var modsToBuild []string
	buildAll := len(modNames) == 0

	if buildAll {
		modsToBuild = getAllMods(cfg)
		if len(modsToBuild) == 0 {
			fmt.Println("No mods found. Create one with 'mithril mod create <name>'.")
			return nil
		}
	} else {
		for _, name := range modNames {
			if _, err := os.Stat(filepath.Join(cfg.ModDir(name), "mod.json")); os.IsNotExist(err) {
				return fmt.Errorf("mod not found: %s", name)
			}
			modsToBuild = append(modsToBuild, name)
		}
	}

	// Ensure build directory exists
	if err := os.MkdirAll(cfg.ModulesBuildDir, 0755); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	// Phase 1: Build DBC binaries and per-mod MPQs
	// Track all built files across mods for the combined MPQ
	var allBuiltFiles []builtFile
	seenDBCs := make(map[string]bool)

	for _, mod := range modsToBuild {
		modFiles, err := buildModDBCs(cfg, mod)
		if err != nil {
			fmt.Printf("  ⚠ Error building mod '%s': %v\n", mod, err)
			continue
		}
		if len(modFiles) == 0 {
			continue
		}

		// Build per-mod MPQ in the build directory.
		// Each mod has an assigned patch slot (A, B, C, ... L, AA, AB, ...)
		// that sorts after patch-3.MPQ but before patch-M.MPQ (the combined).
		modMeta, metaErr := loadModMeta(cfg, mod)
		if metaErr == nil && modMeta.PatchSlot == "" {
			// Backfill slot for mods created before slot assignment existed
			slot, slotErr := nextPatchSlot(cfg)
			if slotErr == nil {
				modMeta.PatchSlot = slot
				data, _ := json.MarshalIndent(modMeta, "", "  ")
				_ = os.WriteFile(filepath.Join(cfg.ModDir(mod), "mod.json"), data, 0644)
				fmt.Printf("  Assigned patch slot %s to mod '%s'\n", slot, mod)
			}
		}
		modMpqName := "patch-" + mod + ".MPQ" // fallback
		if metaErr == nil && modMeta.PatchSlot != "" {
			modMpqName = "patch-" + modMeta.PatchSlot + ".MPQ"
		}
		modMpqPath := filepath.Join(cfg.ModulesBuildDir, modMpqName)
		if err := createMPQ(modMpqPath, modFiles); err != nil {
			fmt.Printf("  ⚠ Failed to create %s: %v\n", modMpqName, err)
		} else {
			fmt.Printf("  ✓ %s (%d DBC file(s))\n", modMpqName, len(modFiles))
		}

		// Add to combined list (later mods override earlier for same DBC)
		for _, bf := range modFiles {
			baseName := strings.TrimSuffix(filepath.Base(bf.diskPath), ".dbc")
			if !seenDBCs[baseName] {
				allBuiltFiles = append(allBuiltFiles, bf)
				seenDBCs[baseName] = true
			}
		}
	}

	if len(allBuiltFiles) == 0 {
		fmt.Println("\nNo modified DBC files to package.")
		return nil
	}

	// Phase 2: Determine the client patch name.
	// - Build all mods: patch-M.MPQ
	// - Build specific mods: patch-<slot1>-<slot2>-...MPQ (slots sorted)
	clientDataDir := filepath.Join(cfg.ClientDir, "Data")
	var clientMpqName string

	if buildAll {
		clientMpqName = "patch-M.MPQ"
	} else {
		// Collect patch slots for the selected mods
		var slots []string
		for _, mod := range modsToBuild {
			modMeta, err := loadModMeta(cfg, mod)
			if err == nil && modMeta.PatchSlot != "" {
				slots = append(slots, modMeta.PatchSlot)
			}
		}
		sort.Strings(slots)
		clientMpqName = "patch-" + strings.Join(slots, "-") + ".MPQ"
	}

	// Build the MPQ in modules/build/
	buildMpqPath := filepath.Join(cfg.ModulesBuildDir, clientMpqName)
	fmt.Printf("\nBuilding %s...\n", clientMpqName)
	if err := createMPQ(buildMpqPath, allBuiltFiles); err != nil {
		return fmt.Errorf("create MPQ: %w", err)
	}

	// Clean all mithril-generated patches from the client Data/ directory,
	// then deploy the new one. This ensures only one mithril patch is active.
	cleanedCount := cleanMithrilPatches(clientDataDir)
	if cleanedCount > 0 {
		fmt.Printf("Cleaned %d previous mithril patch(es) from client\n", cleanedCount)
	}

	clientMpqPath := filepath.Join(clientDataDir, clientMpqName)
	if err := copyFile(buildMpqPath, clientMpqPath); err != nil {
		return fmt.Errorf("deploy to client: %w", err)
	}

	// Phase 3: Deploy modified DBCs to the server's data/dbc/ directory.
	// TrinityCore reads DBC files from flat files on disk (data/dbc/), not from MPQs.
	serverDeployed := 0
	if _, err := os.Stat(cfg.ServerDbcDir); err == nil {
		fmt.Printf("\nDeploying to server (data/dbc/)...\n")
		for _, bf := range allBuiltFiles {
			dbcFileName := filepath.Base(strings.ReplaceAll(bf.mpqPath, "\\", "/"))
			serverPath := filepath.Join(cfg.ServerDbcDir, dbcFileName)
			if err := copyFile(bf.diskPath, serverPath); err != nil {
				fmt.Printf("  ⚠ Failed to deploy %s to server: %v\n", dbcFileName, err)
			} else {
				fmt.Printf("  ✓ %s\n", dbcFileName)
				serverDeployed++
			}
		}
	}

	label := strings.Join(modsToBuild, ", ")
	fmt.Printf("\n=== Build Complete ===\n")
	fmt.Printf("  Mods:     %s\n", label)
	fmt.Println()
	fmt.Println("  Per-mod MPQs (modules/build/):")
	for _, mod := range modsToBuild {
		modMeta, metaErr := loadModMeta(cfg, mod)
		modMpqName := "patch-" + mod + ".MPQ"
		if metaErr == nil && modMeta.PatchSlot != "" {
			modMpqName = "patch-" + modMeta.PatchSlot + ".MPQ"
		}
		modMpqPath := filepath.Join(cfg.ModulesBuildDir, modMpqName)
		if info, err := os.Stat(modMpqPath); err == nil {
			fmt.Printf("    %s  ← %s (%d bytes)\n", modMpqName, mod, info.Size())
		}
	}
	fmt.Println()
	fmt.Printf("  Client:   %s → %s\n", clientMpqName, clientMpqPath)
	if serverDeployed > 0 {
		fmt.Printf("  Server:   %d DBC(s) → %s\n", serverDeployed, cfg.ServerDbcDir)
	}
	fmt.Printf("  Total:    %d DBC(s) packaged\n", len(allBuiltFiles))
	fmt.Println()

	// Show active mithril patches in the client
	activeMithrilPatches := listMithrilPatches(clientDataDir)
	if len(activeMithrilPatches) == 0 {
		fmt.Println("No mithril patches active in client.")
	} else if len(activeMithrilPatches) == 1 {
		fmt.Printf("Active mithril patch: %s\n", activeMithrilPatches[0])
	} else {
		fmt.Println("Active mithril patches:")
		for _, p := range activeMithrilPatches {
			fmt.Printf("  %s\n", p)
		}
	}
	if serverDeployed > 0 {
		fmt.Println()
		fmt.Println("⚠ Server DBC files were updated. Restart the server for changes to take effect:")
		fmt.Println("  mithril server restart")
	}

	return nil
}

// buildModDBCs converts a mod's modified CSVs to DBC binaries and returns the list of built files.
func buildModDBCs(cfg *Config, mod string) ([]builtFile, error) {
	modDbcDir := cfg.ModDbcDir(mod)
	modCSVs, _ := findCSVFiles(modDbcDir)

	if len(modCSVs) == 0 {
		return nil, nil
	}

	modified := findModifiedDBCsInMod(cfg, mod)
	if len(modified) == 0 {
		fmt.Printf("  Mod '%s': no changes from baseline, skipping\n", mod)
		return nil, nil
	}

	fmt.Printf("  Mod '%s': %d modified DBC(s)\n", mod, len(modified))

	buildDbcDir := filepath.Join(cfg.ModulesBuildDir, mod, "DBFilesClient")
	if err := os.MkdirAll(buildDbcDir, 0755); err != nil {
		return nil, fmt.Errorf("create build dir: %w", err)
	}

	var files []builtFile
	for _, baseName := range modified {
		csvPath := filepath.Join(modDbcDir, baseName+".dbc.csv")

		meta, err := dbc.GetMetaForDBC(baseName)
		if err != nil {
			fmt.Printf("    ⚠ No schema for %s, skipping: %v\n", baseName, err)
			continue
		}

		dbcFile, err := dbc.ImportCSV(csvPath, meta)
		if err != nil {
			fmt.Printf("    ⚠ Failed to parse CSV for %s: %v\n", baseName, err)
			continue
		}

		dbcOutPath := filepath.Join(buildDbcDir, baseName+".dbc")
		if err := dbc.WriteDBC(dbcFile, meta, dbcOutPath); err != nil {
			fmt.Printf("    ⚠ Failed to write DBC for %s: %v\n", baseName, err)
			continue
		}

		dbcFileName := strings.ToUpper(string(baseName[0])) + baseName[1:] + ".dbc"
		mpqInternalPath := "DBFilesClient\\" + dbcFileName

		files = append(files, builtFile{diskPath: dbcOutPath, mpqPath: mpqInternalPath})
		fmt.Printf("    ✓ %s (%d records)\n", baseName, dbcFile.Header.RecordCount)
	}

	return files, nil
}

// listMithrilPatches returns the names of all mithril-generated patches in the directory.
func listMithrilPatches(clientDataDir string) []string {
	entries, err := os.ReadDir(clientDataDir)
	if err != nil {
		return nil
	}
	var patches []string
	for _, entry := range entries {
		if !entry.IsDir() && isMithrilPatch(entry.Name()) {
			patches = append(patches, entry.Name())
		}
	}
	sort.Strings(patches)
	return patches
}

// cleanMithrilPatches removes all mithril-generated patch files from the client
// Data/ directory. Mithril patches use letter-based slot names (e.g., patch-A.MPQ,
// patch-M.MPQ, patch-B-C.MPQ). Base game patches use numeric suffixes (patch.MPQ,
// patch-2.MPQ, patch-3.MPQ) or locale names (patch-enUS.MPQ) and are left alone.
func cleanMithrilPatches(clientDataDir string) int {
	entries, err := os.ReadDir(clientDataDir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if isMithrilPatch(name) {
			path := filepath.Join(clientDataDir, name)
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}
	return removed
}

// isMithrilPatch returns true if a filename looks like a mithril-generated patch.
// Mithril patches match: patch-<LETTERS>[-<LETTERS>...].MPQ
// where each segment is uppercase letters only (A-L, M, AA-LL, etc.)
//
// NOT mithril patches:
//   - patch.MPQ (no suffix)
//   - patch-2.MPQ, patch-3.MPQ (numeric)
//   - patch-enUS.MPQ (locale — has lowercase letters)
func isMithrilPatch(filename string) bool {
	// Normalize: compare case-insensitively for the "patch-" prefix and ".MPQ" suffix
	lower := strings.ToLower(filename)
	if !strings.HasPrefix(lower, "patch-") || !strings.HasSuffix(lower, ".mpq") {
		return false
	}
	// Extract the part between "patch-" and ".MPQ"
	middle := filename[6 : len(filename)-4] // strip "patch-" and ".MPQ"
	if middle == "" {
		return false
	}
	// Each segment separated by '-' must be all uppercase letters (A-Z).
	// This distinguishes mithril patches (patch-A.MPQ, patch-M.MPQ, patch-B-C.MPQ)
	// from base game patches:
	//   - patch-2.MPQ, patch-3.MPQ (numeric)
	//   - patch-enUS.MPQ (has lowercase — locale patch)
	segments := strings.Split(middle, "-")
	for _, seg := range segments {
		if seg == "" {
			return false
		}
		for _, c := range seg {
			if c < 'A' || c > 'Z' {
				return false
			}
		}
	}
	return true
}

// createMPQ creates an MPQ archive at the given path containing the given files.
func createMPQ(mpqOutPath string, files []builtFile) error {
	if err := os.MkdirAll(filepath.Dir(mpqOutPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	archive, err := mpq.Create(mpqOutPath, len(files)+2)
	if err != nil {
		return fmt.Errorf("create MPQ: %w", err)
	}

	for _, bf := range files {
		if err := archive.AddFile(bf.diskPath, bf.mpqPath); err != nil {
			return fmt.Errorf("add file %s: %w", bf.mpqPath, err)
		}
	}

	if err := archive.Close(); err != nil {
		return fmt.Errorf("close MPQ: %w", err)
	}

	return nil
}

func runModStatus(args []string) error {
	modName, _ := parseModFlag(args)
	cfg := DefaultConfig()

	manifest, err := loadManifest(cfg.BaselineDir)
	if err != nil {
		return fmt.Errorf("baseline not found — run 'mithril mod init' first")
	}

	fmt.Println("=== Mithril Mod Status ===")
	fmt.Printf("  Baseline extracted: %s\n", manifest.ExtractedAt)
	fmt.Printf("  Locale:             %s\n", manifest.Locale)
	fmt.Printf("  Total baseline DBCs: %d\n", len(manifest.Files))
	fmt.Println()

	// If a specific mod requested
	if modName != "" {
		if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
			return fmt.Errorf("mod not found: %s", modName)
		}
		modified := findModifiedDBCsInMod(cfg, modName)
		if len(modified) == 0 {
			fmt.Printf("Mod '%s': no modifications\n", modName)
		} else {
			fmt.Printf("Mod '%s': %d modified DBC(s)\n", modName, len(modified))
			for _, name := range modified {
				fmt.Printf("  ✏ %s\n", name)
			}
		}
	} else {
		// Show all mods
		mods := getAllMods(cfg)
		if len(mods) == 0 {
			fmt.Println("No mods created. Run 'mithril mod create <name>' to start.")
			return nil
		}

		for _, mod := range mods {
			modified := findModifiedDBCsInMod(cfg, mod)
			if len(modified) == 0 {
				fmt.Printf("  %s: no modifications\n", mod)
			} else {
				fmt.Printf("  %s: %d modified DBC(s)\n", mod, len(modified))
				for _, name := range modified {
					fmt.Printf("    ✏ %s\n", name)
				}
			}
		}
	}

	// Check if patch-M.MPQ exists
	patchPath := filepath.Join(cfg.ClientDir, "Data", "patch-M.MPQ")
	if info, err := os.Stat(patchPath); err == nil {
		fmt.Printf("\nActive patch: %s (%d bytes)\n", filepath.Base(patchPath), info.Size())
	}

	return nil
}

// findModifiedDBCsInMod finds DBCs in a mod that differ from the baseline.
func findModifiedDBCsInMod(cfg *Config, modName string) []string {
	modDbcDir := cfg.ModDbcDir(modName)
	csvFiles, err := findCSVFiles(modDbcDir)
	if err != nil {
		return nil
	}

	var modified []string
	for _, csvPath := range csvFiles {
		baseName := strings.TrimSuffix(filepath.Base(csvPath), ".dbc.csv")
		baselinePath := filepath.Join(cfg.BaselineCsvDir, baseName+".dbc.csv")

		if !filesEqual(csvPath, baselinePath) {
			modified = append(modified, baseName)
		}
	}

	sort.Strings(modified)
	return modified
}

func filesEqual(path1, path2 string) bool {
	data1, err := os.ReadFile(path1)
	if err != nil {
		return false
	}
	data2, err := os.ReadFile(path2)
	if err != nil {
		return false
	}
	h1 := md5.Sum(data1)
	h2 := md5.Sum(data2)
	return hex.EncodeToString(h1[:]) == hex.EncodeToString(h2[:])
}
