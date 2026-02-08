package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/suprsokr/go-mpq"
	"github.com/suprsokr/mithril/internal/dbc"
)

// Manifest tracks the state of the baseline DBC extraction and mod build settings.
type Manifest struct {
	ExtractedAt string   `json:"extracted_at"`
	ClientData  string   `json:"client_data"`
	Locale      string   `json:"locale"`
	MPQChain    []string `json:"mpq_chain"`
	// BuildOrder controls which mods are built and in what order.
	// Mods listed first have lowest priority — later mods override earlier ones
	// when they modify the same file. Mods on disk but not listed here are
	// appended alphabetically after the explicit list.
	// Automatically populated when mods are created or installed.
	// Users can reorder entries in modules/baseline/manifest.json to change priority.
	BuildOrder []string `json:"build_order"`
}

func runModInit(args []string) error {
	cfg := DefaultConfig()

	clientDataDir := filepath.Join(cfg.ClientDir, "Data")
	if _, err := os.Stat(clientDataDir); os.IsNotExist(err) {
		return fmt.Errorf("client Data directory not found: %s\nPlease ensure the WoW 3.3.5a client is in %s", clientDataDir, cfg.ClientDir)
	}

	fmt.Println("=== Mithril Mod Init ===")
	fmt.Printf("Client data: %s\n", clientDataDir)

	// Create output directories
	for _, d := range []string{cfg.ModulesDir, cfg.BaselineDir, cfg.BaselineDbcDir, cfg.BaselineAddonsDir, cfg.ModulesBuildDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Detect locale
	locale := detectLocale(clientDataDir)
	fmt.Printf("Detected locale: %s\n", locale)

	// Find and order MPQ files following TrinityCore's load order
	mpqFiles, err := findDBCMPQs(clientDataDir, locale)
	if err != nil {
		return fmt.Errorf("find MPQ files: %w", err)
	}
	if len(mpqFiles) == 0 {
		return fmt.Errorf("no MPQ files found in %s", clientDataDir)
	}

	fmt.Printf("Found %d MPQ archives in patch chain\n\n", len(mpqFiles))

	// Open all archives
	type archiveInfo struct {
		archive *mpq.Archive
		path    string
	}
	var archives []archiveInfo
	for _, mpqFile := range mpqFiles {
		archive, err := mpq.Open(mpqFile)
		if err != nil {
			fmt.Printf("  ⚠ Skipping %s: %v\n", filepath.Base(mpqFile), err)
			continue
		}
		archives = append(archives, archiveInfo{archive: archive, path: mpqFile})
		fmt.Printf("  ✓ Opened: %s\n", filepath.Base(mpqFile))
	}
	if len(archives) == 0 {
		return fmt.Errorf("no MPQ archives could be opened")
	}

	// Collect all DBC files, later archives override earlier ones
	type dbcSource struct {
		mpqPath    string
		archiveIdx int
	}
	dbcFiles := make(map[string]dbcSource)

	for i := len(archives) - 1; i >= 0; i-- {
		files, err := archives[i].archive.ListFiles()
		if err != nil {
			fmt.Printf("  ⚠ Failed to list files in %s: %v\n", filepath.Base(archives[i].path), err)
			continue
		}

		for _, file := range files {
			fileLower := strings.ToLower(file)
			if strings.HasPrefix(fileLower, "dbfilesclient\\") && strings.HasSuffix(fileLower, ".dbc") {
				normalized := strings.ReplaceAll(file, "\\", "/")
				dbcName := normalizeDBCFilename(filepath.Base(normalized))
				if _, exists := dbcFiles[dbcName]; !exists {
					dbcFiles[dbcName] = dbcSource{mpqPath: file, archiveIdx: i}
				}
			}
		}
	}

	fmt.Printf("\nFound %d unique DBC files across all archives\n", len(dbcFiles))

	// Preserve existing build_order if re-initializing
	var existingBuildOrder []string
	if oldManifest, err := loadManifest(cfg.BaselineDir); err == nil {
		existingBuildOrder = oldManifest.BuildOrder
	}

	// Extract to baseline
	manifest := &Manifest{
		ExtractedAt: timeNow(),
		ClientData:  clientDataDir,
		Locale:      locale,
		MPQChain:    mpqFiles,
		BuildOrder:  existingBuildOrder,
	}

	extracted := 0
	withMeta := 0
	withoutMeta := 0

	dbcNames := make([]string, 0, len(dbcFiles))
	for name := range dbcFiles {
		dbcNames = append(dbcNames, name)
	}
	sort.Strings(dbcNames)

	fmt.Println("\nExtracting baseline DBC files...")
	for _, dbcName := range dbcNames {
		src := dbcFiles[dbcName]
		archive := archives[src.archiveIdx]

		// Extract raw .dbc to baseline/dbc/
		rawPath := filepath.Join(cfg.BaselineDbcDir, dbcName)
		if err := archive.archive.ExtractFile(src.mpqPath, rawPath); err != nil {
			fmt.Printf("  ⚠ Failed to extract %s: %v\n", dbcName, err)
			continue
		}

		// Read to validate and count meta coverage
		rawData, err := os.ReadFile(rawPath)
		if err != nil {
			fmt.Printf("  ⚠ Failed to read %s: %v\n", dbcName, err)
			continue
		}

		// Try to find meta for this DBC
		baseName := strings.TrimSuffix(dbcName, filepath.Ext(dbcName))
		meta, metaErr := dbc.GetMetaForDBC(baseName)

		hasMeta := metaErr == nil
		if hasMeta {
			// Parse with known schema to validate
			_, err := dbc.LoadDBCFromBytes(rawData, *meta)
			if err != nil {
				fmt.Printf("  ⚠ Failed to parse %s (meta mismatch?): %v\n", dbcName, err)
				hasMeta = false
			} else {
				withMeta++
			}
		}

		if !hasMeta {
			withoutMeta++
		}

		extracted++
	}

	// Write manifest
	manifestPath := filepath.Join(cfg.BaselineDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// --- Phase 2: Extract addon files (lua, xml, toc) ---
	fmt.Println("\nExtracting addon files (lua, xml, toc)...")

	// All addon files live in locale archives. Collect them with
	// later archives overriding earlier ones (same as DBC extraction).
	type addonSource struct {
		mpqPath    string
		archiveIdx int
	}
	addonFiles := make(map[string]addonSource)

	for i := len(archives) - 1; i >= 0; i-- {
		files, err := archives[i].archive.ListFiles()
		if err != nil {
			continue
		}
		for _, file := range files {
			lower := strings.ToLower(file)
			if !strings.HasPrefix(lower, "interface") {
				continue
			}
			if !strings.HasSuffix(lower, ".lua") && !strings.HasSuffix(lower, ".xml") && !strings.HasSuffix(lower, ".toc") {
				continue
			}
			normalized := strings.ReplaceAll(file, "\\", "/")
			key := strings.ToLower(normalized)
			if _, exists := addonFiles[key]; !exists {
				addonFiles[key] = addonSource{mpqPath: file, archiveIdx: i}
			}
		}
	}

	addonCount := 0
	for _, src := range addonFiles {
		archive := archives[src.archiveIdx]
		normalized := strings.ReplaceAll(src.mpqPath, "\\", "/")
		outPath := filepath.Join(cfg.BaselineAddonsDir, normalized)

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			fmt.Printf("  ⚠ Failed to create dir for %s: %v\n", normalized, err)
			continue
		}

		if err := archive.archive.ExtractFile(src.mpqPath, outPath); err != nil {
			fmt.Printf("  ⚠ Failed to extract %s: %v\n", normalized, err)
			continue
		}
		addonCount++
	}

	fmt.Printf("  Extracted %d addon files\n", addonCount)

	fmt.Printf("\n=== Extraction Complete ===\n")
	fmt.Printf("  DBC files:          %d (%d with schema, %d raw only)\n", extracted, withMeta, withoutMeta)
	fmt.Printf("  Addon files:        %d (lua/xml/toc)\n", addonCount)
	fmt.Printf("  Baseline DBCs:      %s\n", cfg.BaselineDbcDir)
	fmt.Printf("  Baseline addons:    %s\n", cfg.BaselineAddonsDir)
	fmt.Printf("  Manifest:           %s\n", manifestPath)

	// --- Phase 3: Import DBCs into MySQL for SQL-based editing ---
	fmt.Println("\nImporting DBC data into MySQL...")
	if err := runModDBCImport(nil); err != nil {
		printWarning(fmt.Sprintf("DBC SQL import: %v", err))
		printInfo("You can import later with: mithril mod dbc import")
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  mithril mod create my-mod              # Create a mod")
	fmt.Println("  mithril mod dbc list                   # List all DBCs")
	fmt.Println("  mithril mod dbc search \"Fireball\"       # Search across DBCs")
	fmt.Println("  mithril mod dbc query \"SELECT ...\"      # Query DBC data with SQL")
	fmt.Println("  mithril mod addon list                 # List all addon files")
	fmt.Println("  mithril mod addon search \"pattern\"      # Search addon files")

	return nil
}

// detectLocale finds the locale directory under Data/ (e.g., enUS, deDE).
func detectLocale(dataDir string) string {
	knownLocales := []string{"enUS", "enGB", "deDE", "frFR", "esES", "esMX", "ruRU", "koKR", "zhCN", "zhTW", "ptBR", "itIT"}
	for _, loc := range knownLocales {
		localeDir := filepath.Join(dataDir, loc)
		if info, err := os.Stat(localeDir); err == nil && info.IsDir() {
			return loc
		}
	}
	return "enUS"
}

// findDBCMPQs finds and orders MPQ files following TrinityCore's load order.
func findDBCMPQs(dataDir, locale string) ([]string, error) {
	localeDir := filepath.Join(dataDir, locale)

	patterns := []struct {
		dir      string
		pattern  string
	}{
		// Base files
		{localeDir, "expansion-locale-" + locale + ".MPQ"},
		{localeDir, "locale-" + locale + ".MPQ"},
		{dataDir, "expansion.MPQ"},
		{localeDir, "lichking-locale-" + locale + ".MPQ"},
		{dataDir, "common.MPQ"},
		{dataDir, "lichking.MPQ"},
		{dataDir, "common-2.MPQ"},
		// Patches (ascending priority)
		{localeDir, "patch-" + locale + ".MPQ"},
		{dataDir, "patch.MPQ"},
		{localeDir, "patch-" + locale + "-2.MPQ"},
		{dataDir, "patch-2.MPQ"},
		{localeDir, "patch-" + locale + "-3.MPQ"},
		{dataDir, "patch-3.MPQ"},
	}

	var mpqFiles []string
	for _, p := range patterns {
		path := filepath.Join(p.dir, p.pattern)
		if _, err := os.Stat(path); err == nil {
			mpqFiles = append(mpqFiles, path)
		}
	}

	return mpqFiles, nil
}

// normalizeDBCFilename normalizes a DBC filename.
func normalizeDBCFilename(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = filepath.Base(filename)

	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	baseLower := strings.ToLower(base)
	if len(baseLower) == 0 {
		return filename
	}

	baseNormalized := strings.ToUpper(string(baseLower[0])) + baseLower[1:]
	return baseNormalized + ".dbc"
}

// loadManifest loads the baseline manifest.
func loadManifest(baselineDir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(baselineDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// saveManifest writes the manifest back to disk.
func saveManifest(baselineDir string, manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(baselineDir, "manifest.json"), data, 0644)
}

// addModToBuildOrder appends a mod to the manifest's build_order if not already present.
func addModToBuildOrder(cfg *Config, modName string) error {
	manifest, err := loadManifest(cfg.BaselineDir)
	if err != nil {
		// No manifest yet (baseline not initialized) — silently skip.
		return nil
	}

	// Check if already in the list
	for _, name := range manifest.BuildOrder {
		if name == modName {
			return nil
		}
	}

	manifest.BuildOrder = append(manifest.BuildOrder, modName)
	return saveManifest(cfg.BaselineDir, manifest)
}

// countBaselineDBCs counts .dbc files in the baseline directory.
func countBaselineDBCs(baselineDbcDir string) int {
	entries, err := os.ReadDir(baselineDbcDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".dbc") {
			count++
		}
	}
	return count
}
