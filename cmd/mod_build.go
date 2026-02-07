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
	"github.com/suprsokr/mithril/internal/patcher"
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

	// Phase 1: Build DBC binaries and collect addon files per mod
	var allDbcFiles []builtFile
	var allAddonFiles []builtFile
	seenDBCs := make(map[string]bool)
	seenAddons := make(map[string]bool)

	// Resolve patch slots for all mods up front
	modSlots := make(map[string]string)
	for _, mod := range modsToBuild {
		modMeta, metaErr := loadModMeta(cfg, mod)
		if metaErr == nil && modMeta.PatchSlot == "" {
			slot, slotErr := nextPatchSlot(cfg)
			if slotErr == nil {
				modMeta.PatchSlot = slot
				data, _ := json.MarshalIndent(modMeta, "", "  ")
				_ = os.WriteFile(filepath.Join(cfg.ModDir(mod), "mod.json"), data, 0644)
				fmt.Printf("  Assigned patch slot %s to mod '%s'\n", slot, mod)
			}
		}
		if metaErr == nil && modMeta.PatchSlot != "" {
			modSlots[mod] = modMeta.PatchSlot
		}
	}

	for _, mod := range modsToBuild {
		// Build DBC files
		dbcFiles, err := buildModDBCs(cfg, mod)
		if err != nil {
			fmt.Printf("  ⚠ Error building DBCs for mod '%s': %v\n", mod, err)
		}

		// Collect addon files
		addonFiles := collectModAddons(cfg, mod)

		if len(dbcFiles) == 0 && len(addonFiles) == 0 {
			continue
		}

		// Build per-mod DBC MPQ (non-locale)
		slot := modSlots[mod]
		if len(dbcFiles) > 0 && slot != "" {
			modMpqName := "patch-" + slot + ".MPQ"
			modMpqPath := filepath.Join(cfg.ModulesBuildDir, modMpqName)
			if err := createMPQ(modMpqPath, dbcFiles); err != nil {
				fmt.Printf("  ⚠ Failed to create %s: %v\n", modMpqName, err)
			} else {
				fmt.Printf("  ✓ %s (%d DBC file(s))\n", modMpqName, len(dbcFiles))
			}
		}

		// Build per-mod addon MPQ (locale-specific)
		if len(addonFiles) > 0 && slot != "" {
			locale := detectLocaleFromManifest(cfg)
			modAddonMpqName := "patch-" + locale + "-" + slot + ".MPQ"
			modAddonMpqPath := filepath.Join(cfg.ModulesBuildDir, modAddonMpqName)
			if err := createMPQ(modAddonMpqPath, addonFiles); err != nil {
				fmt.Printf("  ⚠ Failed to create %s: %v\n", modAddonMpqName, err)
			} else {
				fmt.Printf("  ✓ %s (%d addon file(s))\n", modAddonMpqName, len(addonFiles))
			}
		}

		// Add to combined lists
		for _, bf := range dbcFiles {
			key := strings.ToLower(bf.mpqPath)
			if !seenDBCs[key] {
				allDbcFiles = append(allDbcFiles, bf)
				seenDBCs[key] = true
			}
		}
		for _, bf := range addonFiles {
			key := strings.ToLower(bf.mpqPath)
			if !seenAddons[key] {
				allAddonFiles = append(allAddonFiles, bf)
				seenAddons[key] = true
			}
		}
	}

	if len(allDbcFiles) == 0 && len(allAddonFiles) == 0 {
		fmt.Println("\nNo modified files to package.")
		return nil
	}

	// Phase 2: Determine client patch names and deploy.
	clientDataDir := filepath.Join(cfg.ClientDir, "Data")
	locale := detectLocaleFromManifest(cfg)
	clientLocaleDir := filepath.Join(clientDataDir, locale)

	var slotSuffix string
	if buildAll {
		slotSuffix = "M"
	} else {
		var slots []string
		for _, mod := range modsToBuild {
			if s, ok := modSlots[mod]; ok {
				slots = append(slots, s)
			}
		}
		sort.Strings(slots)
		slotSuffix = strings.Join(slots, "-")
	}

	// Clean all mithril patches from both Data/ and Data/<locale>/
	cleanedCount := cleanMithrilPatches(clientDataDir)
	cleanedCount += cleanMithrilPatches(clientLocaleDir)
	if cleanedCount > 0 {
		fmt.Printf("\nCleaned %d previous mithril patch(es) from client\n", cleanedCount)
	}

	// Deploy DBC MPQ to Data/
	if len(allDbcFiles) > 0 {
		dbcMpqName := "patch-" + slotSuffix + ".MPQ"
		buildDbcMpqPath := filepath.Join(cfg.ModulesBuildDir, dbcMpqName)
		fmt.Printf("\nBuilding %s (%d DBC files)...\n", dbcMpqName, len(allDbcFiles))
		if err := createMPQ(buildDbcMpqPath, allDbcFiles); err != nil {
			return fmt.Errorf("create DBC MPQ: %w", err)
		}
		clientDbcMpqPath := filepath.Join(clientDataDir, dbcMpqName)
		if err := copyFile(buildDbcMpqPath, clientDbcMpqPath); err != nil {
			return fmt.Errorf("deploy DBC MPQ: %w", err)
		}
	}

	// Deploy addon MPQ to Data/<locale>/
	if len(allAddonFiles) > 0 {
		addonMpqName := "patch-" + locale + "-" + slotSuffix + ".MPQ"
		buildAddonMpqPath := filepath.Join(cfg.ModulesBuildDir, addonMpqName)
		fmt.Printf("Building %s (%d addon files)...\n", addonMpqName, len(allAddonFiles))
		if err := createMPQ(buildAddonMpqPath, allAddonFiles); err != nil {
			return fmt.Errorf("create addon MPQ: %w", err)
		}
		clientAddonMpqPath := filepath.Join(clientLocaleDir, addonMpqName)
		if err := copyFile(buildAddonMpqPath, clientAddonMpqPath); err != nil {
			return fmt.Errorf("deploy addon MPQ: %w", err)
		}
	}

	// Phase 3: Deploy modified DBCs to the server's data/dbc/ directory.
	serverDeployed := 0
	if _, err := os.Stat(cfg.ServerDbcDir); err == nil && len(allDbcFiles) > 0 {
		fmt.Printf("\nDeploying to server (data/dbc/)...\n")
		for _, bf := range allDbcFiles {
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
	fmt.Println("  Build artifacts (modules/build/):")
	for _, mod := range modsToBuild {
		slot := modSlots[mod]
		if slot != "" {
			dbcPath := filepath.Join(cfg.ModulesBuildDir, "patch-"+slot+".MPQ")
			if info, err := os.Stat(dbcPath); err == nil {
				fmt.Printf("    patch-%s.MPQ  ← %s DBC (%d bytes)\n", slot, mod, info.Size())
			}
			addonPath := filepath.Join(cfg.ModulesBuildDir, "patch-"+locale+"-"+slot+".MPQ")
			if info, err := os.Stat(addonPath); err == nil {
				fmt.Printf("    patch-%s-%s.MPQ  ← %s addons (%d bytes)\n", locale, slot, mod, info.Size())
			}
		}
	}
	fmt.Println()
	if len(allDbcFiles) > 0 {
		fmt.Printf("  Client DBC:    Data/patch-%s.MPQ (%d files)\n", slotSuffix, len(allDbcFiles))
	}
	if len(allAddonFiles) > 0 {
		fmt.Printf("  Client addons: Data/%s/patch-%s-%s.MPQ (%d files)\n", locale, locale, slotSuffix, len(allAddonFiles))
	}
	if serverDeployed > 0 {
		fmt.Printf("  Server:        %d DBC(s) → %s\n", serverDeployed, cfg.ServerDbcDir)
	}
	fmt.Println()

	// Show active mithril patches
	activePatches := listMithrilPatches(clientDataDir)
	activeLocalePatches := listMithrilPatches(clientLocaleDir)
	allActive := append(activePatches, activeLocalePatches...)
	if len(allActive) == 0 {
		fmt.Println("No mithril patches active in client.")
	} else {
		fmt.Println("Active mithril patches:")
		for _, p := range allActive {
			fmt.Printf("  %s\n", p)
		}
	}
	if serverDeployed > 0 {
		fmt.Println()
		fmt.Println("⚠ Server DBC files were updated. Restart the server for changes to take effect:")
		fmt.Println("  mithril server restart")
	}

	// Check if addon files include GlueXML/FrameXML and warn about binary patch
	if len(allAddonFiles) > 0 {
		hasGlueOrFrame := false
		for _, bf := range allAddonFiles {
			lower := strings.ToLower(bf.mpqPath)
			if strings.Contains(lower, "gluexml") || strings.Contains(lower, "framexml") {
				hasGlueOrFrame = true
				break
			}
		}
		if hasGlueOrFrame {
			trackerPath := filepath.Join(cfg.ModulesDir, "binary_patches_applied.json")
			tracker, _ := patcher.LoadTracker(trackerPath)
			if !tracker.IsApplied("allow-custom-gluexml") {
				fmt.Println()
				fmt.Println("⚠ Your mod includes GlueXML/FrameXML changes. The client will crash")
				fmt.Println("  with 'corrupt interface files' unless you apply the binary patch:")
				fmt.Println("  mithril mod patch apply allow-custom-gluexml")
			}
		}
	}

	return nil
}

// collectModAddons returns builtFile entries for addon files modified in a mod.
func collectModAddons(cfg *Config, mod string) []builtFile {
	modifiedAddons := findModifiedAddons(cfg, mod)
	if len(modifiedAddons) == 0 {
		return nil
	}

	fmt.Printf("  Mod '%s': %d modified addon file(s)\n", mod, len(modifiedAddons))

	var files []builtFile
	for _, relPath := range modifiedAddons {
		diskPath := filepath.Join(cfg.ModAddonsDir(mod), relPath)
		// MPQ paths use backslashes
		mpqPath := strings.ReplaceAll(relPath, "/", "\\")
		files = append(files, builtFile{diskPath: diskPath, mpqPath: mpqPath})
		fmt.Printf("    ✓ %s\n", relPath)
	}
	return files
}

// detectLocaleFromManifest reads the locale from the baseline manifest, with fallback.
func detectLocaleFromManifest(cfg *Config) string {
	manifest, err := loadManifest(cfg.BaselineDir)
	if err == nil && manifest.Locale != "" {
		return manifest.Locale
	}
	return "enUS"
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

// cleanMithrilPatches removes all mithril-generated patch files from the given
// directory. Works for both Data/ and Data/<locale>/.
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
// Mithril patches come in two forms:
//   - Non-locale: patch-<SLOTS>.MPQ  (e.g., patch-A.MPQ, patch-M.MPQ, patch-B-C.MPQ)
//   - Locale:     patch-<locale>-<SLOTS>.MPQ  (e.g., patch-enUS-M.MPQ, patch-enUS-A-B.MPQ)
//
// NOT mithril patches:
//   - patch.MPQ (no suffix)
//   - patch-2.MPQ, patch-3.MPQ (numeric)
//   - patch-enUS.MPQ, patch-enUS-2.MPQ (base locale patches)
func isMithrilPatch(filename string) bool {
	lower := strings.ToLower(filename)
	if !strings.HasPrefix(lower, "patch-") || !strings.HasSuffix(lower, ".mpq") {
		return false
	}
	middle := filename[6 : len(filename)-4] // strip "patch-" and ".MPQ"
	if middle == "" {
		return false
	}

	segments := strings.Split(middle, "-")

	// Check if first segment is a known locale (e.g., "enUS").
	// If so, strip it and check the rest as slot segments.
	knownLocales := map[string]bool{
		"enUS": true, "enGB": true, "deDE": true, "frFR": true,
		"esES": true, "esMX": true, "ruRU": true, "koKR": true,
		"zhCN": true, "zhTW": true, "ptBR": true, "itIT": true,
	}
	if knownLocales[segments[0]] {
		segments = segments[1:]
		if len(segments) == 0 {
			return false // just "patch-enUS.MPQ" — base game
		}
	}

	// Each remaining segment must be all uppercase letters (A-Z).
	for _, seg := range segments {
		if seg == "" {
			return false
		}
		for _, c := range seg {
			if c < 'A' || c > 'Z' {
				return false // has digits or lowercase — not ours
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

	sqlTracker, _ := loadSQLTracker(cfg)
	coreTracker, _ := loadCoreTracker(cfg)

	// Helper to print status for one mod
	printModStatus := func(mod string) {
		modifiedDBCs := findModifiedDBCsInMod(cfg, mod)
		modifiedAddons := findModifiedAddons(cfg, mod)
		sqlMigrations := findMigrations(cfg, mod)
		corePatches := findCorePatches(cfg, mod)

		if len(modifiedDBCs) == 0 && len(modifiedAddons) == 0 && len(sqlMigrations) == 0 && len(corePatches) == 0 {
			fmt.Printf("  %s: no modifications\n", mod)
			return
		}

		slot := ""
		if meta, err := loadModMeta(cfg, mod); err == nil && meta.PatchSlot != "" {
			slot = " (patch-" + meta.PatchSlot + ")"
		}
		fmt.Printf("  %s%s:\n", mod, slot)
		for _, name := range modifiedDBCs {
			fmt.Printf("    ✏ dbc: %s\n", name)
		}
		for _, name := range modifiedAddons {
			fmt.Printf("    ✏ addon: %s\n", name)
		}
		for _, m := range sqlMigrations {
			status := "pending"
			if sqlTracker.IsApplied(m.mod, m.filename) {
				status = "applied"
			}
			fmt.Printf("    📋 sql [%s]: %s/%s\n", status, m.database, m.filename)
		}
		for _, p := range corePatches {
			status := "pending"
			if coreTracker.IsApplied(p.mod, p.filename) {
				status = "applied"
			}
			fmt.Printf("    🔧 core [%s]: %s\n", status, p.filename)
		}
	}

	if modName != "" {
		if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
			return fmt.Errorf("mod not found: %s", modName)
		}
		printModStatus(modName)
	} else {
		mods := getAllMods(cfg)
		if len(mods) == 0 {
			fmt.Println("No mods created. Run 'mithril mod create <name>' to start.")
			return nil
		}
		for _, mod := range mods {
			printModStatus(mod)
		}
	}

	// Show active mithril patches
	clientDataDir := filepath.Join(cfg.ClientDir, "Data")
	locale := detectLocaleFromManifest(cfg)
	clientLocaleDir := filepath.Join(clientDataDir, locale)
	activePatches := listMithrilPatches(clientDataDir)
	activeLocalePatches := listMithrilPatches(clientLocaleDir)
	allActive := append(activePatches, activeLocalePatches...)
	if len(allActive) > 0 {
		fmt.Println("\nActive mithril patches:")
		for _, p := range allActive {
			fmt.Printf("  %s\n", p)
		}
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
