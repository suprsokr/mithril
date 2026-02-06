package cmd

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/suprsokr/mithril/internal/dbc"
)

func runModDBC(subcmd string, args []string) error {
	switch subcmd {
	case "list":
		return runModDBCList(args)
	case "search":
		return runModDBCSearch(args)
	case "inspect":
		return runModDBCInspect(args)
	case "edit":
		return runModDBCEdit(args)
	case "set":
		return runModDBCSet(args)
	case "-h", "--help", "help":
		fmt.Print(modUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod dbc command: %s", subcmd)
	}
}

func runModDBCList(args []string) error {
	cfg := DefaultConfig()

	csvFiles, err := findCSVFiles(cfg.BaselineCsvDir)
	if err != nil || len(csvFiles) == 0 {
		fmt.Println("No baseline DBC CSV files found. Run 'mithril mod init' first.")
		return nil
	}

	manifest, _ := loadManifest(cfg.BaselineDir)

	fmt.Printf("%-35s %8s %8s\n", "DBC Name", "Records", "Fields")
	fmt.Println(strings.Repeat("-", 55))

	sort.Strings(csvFiles)
	for _, csvFile := range csvFiles {
		baseName := strings.TrimSuffix(filepath.Base(csvFile), ".dbc.csv")
		dbcName := baseName + ".dbc"

		records := "?"
		fields := "?"

		if manifest != nil {
			if mf, ok := manifest.Files[dbcName]; ok {
				records = fmt.Sprintf("%d", mf.RecordCount)
				fields = fmt.Sprintf("%d", mf.FieldCount)
			}
		}

		fmt.Printf("%-35s %8s %8s\n", baseName, records, fields)
	}

	fmt.Printf("\nTotal: %d DBC files with known schemas\n", len(csvFiles))

	// Count raw-only DBCs
	rawFiles, _ := findRawDBCFiles(cfg.BaselineDbcDir)
	rawOnly := 0
	for _, rf := range rawFiles {
		baseName := strings.TrimSuffix(filepath.Base(rf), ".dbc")
		csvPath := filepath.Join(cfg.BaselineCsvDir, baseName+".dbc.csv")
		if _, err := os.Stat(csvPath); os.IsNotExist(err) {
			rawOnly++
		}
	}
	if rawOnly > 0 {
		fmt.Printf("      %d additional DBC files without schemas (raw only)\n", rawOnly)
	}

	return nil
}

func runModDBCSearch(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 {
		return fmt.Errorf("usage: mithril mod dbc search <pattern> [--mod <name>]")
	}

	cfg := DefaultConfig()
	pattern := remaining[0]

	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Determine which directory to search
	searchDir := cfg.BaselineCsvDir
	label := "baseline"
	if modName != "" {
		searchDir = cfg.ModDbcDir(modName)
		label = modName
		// Also search baseline for DBCs not overridden by this mod
	}

	if modName != "" {
		// Search mod's overridden files first, then baseline for the rest
		fmt.Printf("Searching mod '%s' (overrides + baseline)...\n", modName)
		modCSVs, _ := findCSVFiles(searchDir)
		modNames := make(map[string]bool)
		totalMatches := 0

		for _, csvFile := range modCSVs {
			baseName := strings.TrimSuffix(filepath.Base(csvFile), ".dbc.csv")
			modNames[baseName] = true
			matches := searchCSVFile(csvFile, re)
			if len(matches) > 0 {
				fmt.Printf("\n=== %s [%s] (%d matches) ===\n", baseName, modName, len(matches))
				for _, m := range matches {
					fmt.Println(m)
				}
				totalMatches += len(matches)
			}
		}

		// Search baseline for non-overridden files
		baseCSVs, _ := findCSVFiles(cfg.BaselineCsvDir)
		for _, csvFile := range baseCSVs {
			baseName := strings.TrimSuffix(filepath.Base(csvFile), ".dbc.csv")
			if modNames[baseName] {
				continue // already searched mod's version
			}
			matches := searchCSVFile(csvFile, re)
			if len(matches) > 0 {
				fmt.Printf("\n=== %s [baseline] (%d matches) ===\n", baseName, len(matches))
				for _, m := range matches {
					fmt.Println(m)
				}
				totalMatches += len(matches)
			}
		}

		if totalMatches == 0 {
			fmt.Printf("No matches found for pattern: %s\n", pattern)
		} else {
			fmt.Printf("\nTotal: %d matches\n", totalMatches)
		}
	} else {
		// Search baseline only
		csvFiles, err := findCSVFiles(searchDir)
		if err != nil || len(csvFiles) == 0 {
			fmt.Printf("No CSV files in %s. Run 'mithril mod init' first.\n", label)
			return nil
		}

		totalMatches := 0
		sort.Strings(csvFiles)
		for _, csvFile := range csvFiles {
			baseName := strings.TrimSuffix(filepath.Base(csvFile), ".dbc.csv")
			matches := searchCSVFile(csvFile, re)
			if len(matches) > 0 {
				fmt.Printf("\n=== %s (%d matches) ===\n", baseName, len(matches))
				for _, m := range matches {
					fmt.Println(m)
				}
				totalMatches += len(matches)
			}
		}

		if totalMatches == 0 {
			fmt.Printf("No matches found for pattern: %s\n", pattern)
		} else {
			fmt.Printf("\nTotal: %d matches across all DBC files\n", totalMatches)
		}
	}

	return nil
}

func runModDBCInspect(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mithril mod dbc inspect <name>")
	}

	cfg := DefaultConfig()
	name := args[0]
	name = strings.TrimSuffix(name, ".dbc.csv")
	name = strings.TrimSuffix(name, ".dbc")

	meta, metaErr := dbc.GetMetaForDBC(name)

	csvPath := filepath.Join(cfg.BaselineCsvDir, name+".dbc.csv")
	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		csvPath = findCSVCaseInsensitive(cfg.BaselineCsvDir, name)
		if csvPath == "" {
			return fmt.Errorf("DBC not found: %s (run 'mithril mod init' first)", name)
		}
	}

	fmt.Printf("=== %s ===\n\n", name)

	if metaErr == nil {
		fmt.Println("Schema (from embedded meta):")
		fmt.Printf("  DBC File:     %s\n", meta.File)
		fmt.Printf("  Primary Keys: %s\n", strings.Join(meta.PrimaryKeys, ", "))
		fmt.Printf("  Fields:       %d\n\n", len(meta.Fields))

		fmt.Printf("  %-30s %-10s %s\n", "Name", "Type", "Count")
		fmt.Println("  " + strings.Repeat("-", 55))
		for _, f := range meta.Fields {
			count := ""
			if f.Count > 1 {
				count = fmt.Sprintf("%d", f.Count)
			}
			fmt.Printf("  %-30s %-10s %s\n", f.Name, f.Type, count)
		}
		fmt.Println()
	} else {
		fmt.Printf("  No embedded schema found for %s\n\n", name)
	}

	f, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open CSV: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("read CSV header: %w", err)
	}

	allRecords, _ := r.ReadAll()
	totalRecords := len(allRecords)

	fmt.Printf("CSV Columns (%d):\n", len(header))
	for i, h := range header {
		if i < 20 {
			fmt.Printf("  [%d] %s\n", i, h)
		}
	}
	if len(header) > 20 {
		fmt.Printf("  ... and %d more columns\n", len(header)-20)
	}

	fmt.Printf("\nRecords: %d\n", totalRecords)

	sampleCount := 5
	if totalRecords < sampleCount {
		sampleCount = totalRecords
	}

	if sampleCount > 0 {
		fmt.Printf("\nFirst %d records (showing first 6 columns):\n\n", sampleCount)
		showCols := 6
		if len(header) < showCols {
			showCols = len(header)
		}

		for i := 0; i < showCols; i++ {
			fmt.Printf("%-20s", truncate(header[i], 18))
		}
		fmt.Println()
		fmt.Println(strings.Repeat("-", showCols*20))

		for i := 0; i < sampleCount; i++ {
			for j := 0; j < showCols; j++ {
				val := ""
				if j < len(allRecords[i]) {
					val = allRecords[i][j]
				}
				fmt.Printf("%-20s", truncate(val, 18))
			}
			fmt.Println()
		}
	}

	return nil
}

func runModDBCEdit(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod dbc edit <name> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	name := remaining[0]
	name = strings.TrimSuffix(name, ".dbc.csv")
	name = strings.TrimSuffix(name, ".dbc")

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Ensure the mod has a copy of this DBC — copy from baseline if not
	modCsvPath := filepath.Join(cfg.ModDbcDir(modName), name+".dbc.csv")
	if _, err := os.Stat(modCsvPath); os.IsNotExist(err) {
		if err := copyBaselineToMod(cfg, modName, name); err != nil {
			return err
		}
		fmt.Printf("Copied %s from baseline to mod '%s'\n", name+".dbc.csv", modName)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		for _, e := range []string{"code", "vim", "nano", "vi"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		fmt.Printf("CSV file is at: %s\n", modCsvPath)
		fmt.Println("Set $EDITOR to your preferred editor and try again.")
		return nil
	}

	fmt.Printf("Opening %s in %s (mod: %s)...\n", name+".dbc.csv", editor, modName)

	cmd := exec.Command(editor, modCsvPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	fmt.Println("File saved.")
	fmt.Printf("Run 'mithril mod build --mod %s' to build the patch MPQ.\n", modName)

	return nil
}

// runModDBCSet programmatically edits a DBC CSV field value.
func runModDBCSet(args []string) error {
	if len(args) < 7 {
		fmt.Println(`Usage: mithril mod dbc set <dbc_name> --mod <mod_name> --where <key>=<value> --set <col>=<value> [--set ...]

Examples:
  mithril mod dbc set Spell --mod my-mod --where id=133 --set spell_name_enUS="Mithril Bolt"
  mithril mod dbc set Spell --mod my-mod --where id=133 --set spell_name_enUS="Inferno Ball" --set spell_name_deDE="Infernoball"`)
		return fmt.Errorf("not enough arguments")
	}

	cfg := DefaultConfig()
	dbcName := args[0]
	dbcName = strings.TrimSuffix(dbcName, ".dbc.csv")
	dbcName = strings.TrimSuffix(dbcName, ".dbc")

	// Parse flags
	var modName, whereKey, whereVal string
	sets := make(map[string]string)

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--mod":
			if i+1 >= len(args) {
				return fmt.Errorf("--mod requires a name")
			}
			i++
			modName = args[i]
		case "--where":
			if i+1 >= len(args) {
				return fmt.Errorf("--where requires a key=value argument")
			}
			i++
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("--where value must be key=value, got: %s", args[i])
			}
			whereKey = parts[0]
			whereVal = parts[1]
		case "--set":
			if i+1 >= len(args) {
				return fmt.Errorf("--set requires a col=value argument")
			}
			i++
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("--set value must be col=value, got: %s", args[i])
			}
			sets[parts[0]] = parts[1]
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if modName == "" {
		return fmt.Errorf("--mod is required")
	}
	if whereKey == "" {
		return fmt.Errorf("--where is required")
	}
	if len(sets) == 0 {
		return fmt.Errorf("at least one --set is required")
	}

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Ensure the mod has a copy of this DBC — copy from baseline if not
	modCsvPath := filepath.Join(cfg.ModDbcDir(modName), dbcName+".dbc.csv")
	if _, err := os.Stat(modCsvPath); os.IsNotExist(err) {
		if err := copyBaselineToMod(cfg, modName, dbcName); err != nil {
			return err
		}
		fmt.Printf("Copied %s from baseline to mod '%s'\n", dbcName+".dbc.csv", modName)
	}

	// Read CSV
	f, err := os.Open(modCsvPath)
	if err != nil {
		return fmt.Errorf("open CSV: %w", err)
	}

	r := csv.NewReader(f)
	r.LazyQuotes = true
	allRows, err := r.ReadAll()
	f.Close()
	if err != nil {
		return fmt.Errorf("read CSV: %w", err)
	}

	if len(allRows) < 2 {
		return fmt.Errorf("CSV has no data rows")
	}

	header := allRows[0]

	colIdx := make(map[string]int)
	for i, h := range header {
		colIdx[h] = i
	}

	whereIdx, ok := colIdx[whereKey]
	if !ok {
		return fmt.Errorf("column %q not found in %s. Available: %s",
			whereKey, dbcName, strings.Join(header[:minInt(len(header), 10)], ", ")+"...")
	}

	for col := range sets {
		if _, ok := colIdx[col]; !ok {
			return fmt.Errorf("column %q not found in %s. Available: %s",
				col, dbcName, strings.Join(header[:minInt(len(header), 10)], ", ")+"...")
		}
	}

	matchCount := 0
	for i := 1; i < len(allRows); i++ {
		if allRows[i][whereIdx] == whereVal {
			matchCount++
			for col, val := range sets {
				idx := colIdx[col]
				oldVal := allRows[i][idx]
				allRows[i][idx] = val
				fmt.Printf("  Row %d: %s = %q → %q\n", i, col, oldVal, val)
			}
		}
	}

	if matchCount == 0 {
		return fmt.Errorf("no rows matched %s=%s in %s", whereKey, whereVal, dbcName)
	}

	out, err := os.Create(modCsvPath)
	if err != nil {
		return fmt.Errorf("write CSV: %w", err)
	}
	defer out.Close()

	w := csv.NewWriter(out)
	if err := w.WriteAll(allRows); err != nil {
		return fmt.Errorf("write CSV: %w", err)
	}
	w.Flush()

	fmt.Printf("\n✓ Updated %d row(s) in %s (mod: %s)\n", matchCount, dbcName+".dbc.csv", modName)
	fmt.Printf("Run 'mithril mod build --mod %s' to package into patch-M.MPQ\n", modName)

	return nil
}

// --- Helper functions ---

// copyBaselineToMod copies a baseline CSV into a mod's dbc directory.
func copyBaselineToMod(cfg *Config, modName, dbcName string) error {
	baselinePath := filepath.Join(cfg.BaselineCsvDir, dbcName+".dbc.csv")
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		baselinePath = findCSVCaseInsensitive(cfg.BaselineCsvDir, dbcName)
		if baselinePath == "" {
			return fmt.Errorf("DBC %q not found in baseline (run 'mithril mod init' first)", dbcName)
		}
	}

	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return fmt.Errorf("read baseline CSV: %w", err)
	}

	modDbcDir := cfg.ModDbcDir(modName)
	if err := os.MkdirAll(modDbcDir, 0755); err != nil {
		return fmt.Errorf("create mod dbc dir: %w", err)
	}

	destPath := filepath.Join(modDbcDir, dbcName+".dbc.csv")
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("write mod CSV: %w", err)
	}

	return nil
}

func findCSVFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".dbc.csv") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

func findRawDBCFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".dbc") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

func findCSVCaseInsensitive(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	nameLower := strings.ToLower(name)
	for _, entry := range entries {
		base := strings.TrimSuffix(entry.Name(), ".dbc.csv")
		if strings.ToLower(base) == nameLower {
			return filepath.Join(dir, entry.Name())
		}
	}
	return ""
}

func searchCSVFile(csvPath string, re *regexp.Regexp) []string {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var matches []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			if lineNum == 1 {
				continue
			}
			display := line
			if len(display) > 200 {
				display = display[:200] + "..."
			}
			matches = append(matches, fmt.Sprintf("  row %d: %s", lineNum-1, display))
			if len(matches) >= 20 {
				matches = append(matches, "  ... (showing first 20 matches)")
				break
			}
		}
	}

	return matches
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
