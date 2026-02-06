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

// Manifest tracks the state of the baseline DBC extraction.
type Manifest struct {
	ExtractedAt string                   `json:"extracted_at"`
	ClientData  string                   `json:"client_data"`
	Locale      string                   `json:"locale"`
	MPQChain    []string                 `json:"mpq_chain"`
	Files       map[string]*ManifestFile `json:"files"`
}

// ManifestFile tracks an individual DBC file in the baseline.
type ManifestFile struct {
	SourceMPQ   string `json:"source_mpq"`
	OriginalMD5 string `json:"original_md5"`
	HasMeta     bool   `json:"has_meta"`
	RecordCount uint32 `json:"record_count"`
	FieldCount  uint32 `json:"field_count"`
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
	for _, d := range []string{cfg.ModulesDir, cfg.BaselineDir, cfg.BaselineDbcDir, cfg.BaselineCsvDir, cfg.ModulesBuildDir} {
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

	// Extract to baseline
	manifest := &Manifest{
		ExtractedAt: timeNow(),
		ClientData:  clientDataDir,
		Locale:      locale,
		MPQChain:    mpqFiles,
		Files:       make(map[string]*ManifestFile),
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

		// Compute MD5 of raw file
		rawData, err := os.ReadFile(rawPath)
		if err != nil {
			fmt.Printf("  ⚠ Failed to read %s: %v\n", dbcName, err)
			continue
		}
		hash := md5.Sum(rawData)
		md5Hex := hex.EncodeToString(hash[:])

		// Try to find meta for this DBC
		baseName := strings.TrimSuffix(dbcName, filepath.Ext(dbcName))
		meta, metaErr := dbc.GetMetaForDBC(baseName)

		mf := &ManifestFile{
			SourceMPQ:   filepath.Base(archive.path),
			OriginalMD5: md5Hex,
			HasMeta:     metaErr == nil,
		}

		if metaErr == nil {
			// Parse with known schema and export to baseline CSV
			dbcFile, err := dbc.LoadDBCFromBytes(rawData, *meta)
			if err != nil {
				fmt.Printf("  ⚠ Failed to parse %s (meta mismatch?): %v\n", dbcName, err)
				mf.HasMeta = false
			} else {
				csvName := baseName + ".dbc.csv"
				csvPath := filepath.Join(cfg.BaselineCsvDir, csvName)
				if err := dbc.ExportCSV(&dbcFile, meta, csvPath); err != nil {
					fmt.Printf("  ⚠ Failed to export CSV for %s: %v\n", dbcName, err)
				} else {
					mf.RecordCount = dbcFile.Header.RecordCount
					mf.FieldCount = dbcFile.Header.FieldCount
					withMeta++
				}
			}
		}

		if !mf.HasMeta {
			if len(rawData) >= 20 {
				header, err := dbc.ParseHeader(rawData[:20])
				if err == nil {
					mf.RecordCount = header.RecordCount
					mf.FieldCount = header.FieldCount
				}
			}
			withoutMeta++
		}

		manifest.Files[dbcName] = mf
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

	fmt.Printf("\n=== Extraction Complete ===\n")
	fmt.Printf("  Total DBC files:    %d\n", extracted)
	fmt.Printf("  With known schema:  %d (exported to CSV)\n", withMeta)
	fmt.Printf("  Without schema:     %d (raw .dbc only)\n", withoutMeta)
	fmt.Printf("  Baseline CSVs:      %s\n", cfg.BaselineCsvDir)
	fmt.Printf("  Baseline raw DBCs:  %s\n", cfg.BaselineDbcDir)
	fmt.Printf("  Manifest:           %s\n", manifestPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  mithril mod create my-mod          # Create a mod")
	fmt.Println("  mithril mod dbc list               # List all DBCs")
	fmt.Println("  mithril mod dbc search \"Fireball\"   # Search across DBCs")

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
