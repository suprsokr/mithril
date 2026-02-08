package cmd

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/suprsokr/go-mpq"
	"github.com/suprsokr/mithril/internal/dbc"
)

// builtFile tracks a file ready for MPQ packaging.
type builtFile struct {
	diskPath string // path to the .dbc binary on disk
	mpqPath  string // path inside the MPQ (e.g., "DBFilesClient\Spell.dbc")
}

func runModBuild(args []string) error {
	cfg := DefaultConfig()

	// Ensure baseline exists
	if _, err := os.Stat(cfg.BaselineDbcDir); os.IsNotExist(err) {
		return fmt.Errorf("baseline not found — run 'mithril mod init' first")
	}

	fmt.Println("=== Mithril Mod Build ===")

	// Always build all mods
	modsToBuild := getAllMods(cfg)
	if len(modsToBuild) == 0 {
		fmt.Println("No mods found. Create one with 'mithril mod create <name>'.")
		return nil
	}

	// Ensure build directory exists
	if err := os.MkdirAll(cfg.ModulesBuildDir, 0755); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	// Phase 1: Build DBC binaries and collect addon files from all mods
	var allDbcFiles []builtFile
	var allAddonFiles []builtFile
	seenDBCs := make(map[string]bool)
	seenAddons := make(map[string]bool)

	for _, mod := range modsToBuild {
		fmt.Printf("  Mod '%s':\n", mod)

		// Build DBC files (SQL-based) — apply sql/dbc/ migrations and export
		dbcFiles, err := buildModDBCsFromSQL(cfg, mod)
		if err != nil {
			fmt.Printf("  ⚠ Error building DBCs for mod '%s': %v\n", mod, err)
		}

		// Collect addon files
		addonFiles := collectModAddons(cfg, mod)

		if len(dbcFiles) == 0 && len(addonFiles) == 0 {
			continue
		}

		// Add to combined lists (later mods override earlier ones for the same file)
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

	// Phase 2: Build and deploy combined MPQs.
	clientDataDir := filepath.Join(cfg.ClientDir, "Data")
	locale := detectLocaleFromManifest(cfg)
	clientLocaleDir := filepath.Join(clientDataDir, locale)

	// Clean all mithril patches from both Data/ and Data/<locale>/
	cleanedCount := cleanMithrilPatches(clientDataDir)
	cleanedCount += cleanMithrilPatches(clientLocaleDir)
	if cleanedCount > 0 {
		fmt.Printf("\nCleaned %d previous mithril patch(es) from client\n", cleanedCount)
	}

	// Deploy DBC MPQ to Data/
	if len(allDbcFiles) > 0 {
		dbcMpqName := "patch-" + cfg.PatchLetter + ".MPQ"
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
		addonMpqName := "patch-" + locale + "-" + cfg.PatchLetter + ".MPQ"
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
	if len(allDbcFiles) > 0 {
		fmt.Printf("  Client DBC:    Data/patch-%s.MPQ (%d files)\n", cfg.PatchLetter, len(allDbcFiles))
	}
	if len(allAddonFiles) > 0 {
		fmt.Printf("  Client addons: Data/%s/patch-%s-%s.MPQ (%d files)\n", locale, locale, cfg.PatchLetter, len(allAddonFiles))
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

	return nil
}

// collectModAddons returns builtFile entries for addon files modified in a mod.
func collectModAddons(cfg *Config, mod string) []builtFile {
	modifiedAddons := findModifiedAddons(cfg, mod)
	if len(modifiedAddons) == 0 {
		return nil
	}

	fmt.Printf("    %d modified addon file(s)\n", len(modifiedAddons))

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
	fmt.Printf("  Total baseline DBCs: %d\n", countBaselineDBCs(cfg.BaselineDbcDir))
	fmt.Println()

	sqlTracker, _ := loadSQLTracker(cfg)
	coreTracker, _ := loadCoreTracker(cfg)

	// Helper to print status for one mod
	printModStatus := func(mod string) {
		modifiedAddons := findModifiedAddons(cfg, mod)
		sqlMigrations := findMigrations(cfg, mod)
		corePatches := findCorePatches(cfg, mod)

		if len(modifiedAddons) == 0 && len(sqlMigrations) == 0 && len(corePatches) == 0 {
			fmt.Printf("  %s: no modifications\n", mod)
			return
		}

		fmt.Printf("  %s:\n", mod)
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


// buildModDBCsFromSQL applies a mod's sql/dbc/ migrations and exports modified DBC tables.
// Uses native MySQL driver for both migration execution and DBC export.
// Uses CHECKSUM TABLE to detect which tables actually changed.
func buildModDBCsFromSQL(cfg *Config, mod string) ([]builtFile, error) {
	// Check if this mod has any dbc SQL migrations
	dbcMigrations := findDBCMigrations(cfg, mod)
	if len(dbcMigrations) == 0 {
		return nil, nil
	}

	// Open connection to dbc database
	db, err := openDBCDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to dbc database: %w", err)
	}
	defer db.Close()

	// Apply pending DBC SQL migrations
	tracker, _ := loadSQLTracker(cfg)
	applied := 0
	for _, m := range dbcMigrations {
		if tracker.IsApplied(m.mod, m.filename) {
			continue
		}

		fmt.Printf("    Applying DBC SQL: %s ...\n", m.filename)
		sqlContent, err := os.ReadFile(m.path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", m.filename, err)
		}

		if _, err := db.Exec(string(sqlContent)); err != nil {
			return nil, fmt.Errorf("apply migration %s: %w", m.filename, err)
		}

		tracker.Applied = append(tracker.Applied, AppliedMigration{
			Mod:       m.mod,
			File:      m.filename,
			Database:  "dbc",
			AppliedAt: timeNow(),
		})
		applied++
		fmt.Printf("    ✓ %s\n", m.filename)
	}

	if applied > 0 {
		if err := saveSQLTracker(cfg, tracker); err != nil {
			fmt.Printf("    ⚠ Failed to save migration tracker: %v\n", err)
		}
	}

	// Export modified DBC tables using CHECKSUM TABLE for change detection
	metaFiles, err := dbc.GetEmbeddedMetaFiles()
	if err != nil {
		return nil, fmt.Errorf("get meta files: %w", err)
	}

	buildDbcDir := filepath.Join(cfg.ModulesBuildDir, mod, "DBFilesClient")
	if err := os.MkdirAll(buildDbcDir, 0755); err != nil {
		return nil, fmt.Errorf("create build dir: %w", err)
	}

	exported, err := dbc.ExportModifiedDBCs(db, metaFiles, cfg.BaselineDbcDir, buildDbcDir)
	if err != nil {
		return nil, fmt.Errorf("export modified DBCs: %w", err)
	}

	// Build the file list from exported tables
	var files []builtFile
	for _, tableName := range exported {
		// Find the meta to get the original .dbc filename
		for _, metaFile := range metaFiles {
			meta, err := dbc.LoadEmbeddedMeta(metaFile)
			if err != nil {
				continue
			}
			if dbc.TableName(meta) == tableName {
				dbcOutPath := filepath.Join(buildDbcDir, meta.File)
				mpqInternalPath := "DBFilesClient\\" + meta.File
				files = append(files, builtFile{diskPath: dbcOutPath, mpqPath: mpqInternalPath})
				break
			}
		}
	}

	return files, nil
}

// findDBCMigrations returns SQL migrations specifically for the dbc database.
func findDBCMigrations(cfg *Config, modName string) []migrationInfo {
	allMigrations := findMigrations(cfg, modName)
	var dbcMigrations []migrationInfo
	for _, m := range allMigrations {
		if m.database == "dbc" {
			dbcMigrations = append(dbcMigrations, m)
		}
	}
	return dbcMigrations
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
