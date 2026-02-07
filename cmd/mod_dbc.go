package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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
	case "import":
		return runModDBCImport(args)
	case "query":
		return runModDBCQuery(args)
	case "export":
		return runModDBCExport(args)
	case "-h", "--help", "help":
		fmt.Print(modUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod dbc command: %s", subcmd)
	}
}

func runModDBCList(args []string) error {
	cfg := DefaultConfig()

	db, err := openDBCDB(cfg)
	if err != nil {
		return fmt.Errorf("connect to dbc database: %w (run 'mithril mod init' first)", err)
	}
	defer db.Close()

	metaFiles, err := dbc.GetEmbeddedMetaFiles()
	if err != nil {
		return fmt.Errorf("get meta files: %w", err)
	}

	fmt.Printf("%-35s %8s %8s\n", "DBC Name", "Records", "Fields")
	fmt.Println(strings.Repeat("-", 55))

	listed := 0
	for _, metaFile := range metaFiles {
		meta, err := dbc.LoadEmbeddedMeta(metaFile)
		if err != nil {
			continue
		}

		tableName := dbc.TableName(meta)

		var count int
		err = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)).Scan(&count)
		if err != nil {
			continue
		}

		fieldCount := dbc.FieldCount(meta)
		fmt.Printf("%-35s %8d %8d\n", tableName, count, fieldCount)
		listed++
	}

	fmt.Printf("\nTotal: %d DBC tables\n", listed)
	return nil
}

func runModDBCSearch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mithril mod dbc search <pattern>")
	}

	cfg := DefaultConfig()
	pattern := args[0]

	db, err := openDBCDB(cfg)
	if err != nil {
		return fmt.Errorf("connect to dbc database: %w (run 'mithril mod init' first)", err)
	}
	defer db.Close()

	metaFiles, err := dbc.GetEmbeddedMetaFiles()
	if err != nil {
		return fmt.Errorf("get meta files: %w", err)
	}

	totalMatches := 0
	for _, metaFile := range metaFiles {
		meta, err := dbc.LoadEmbeddedMeta(metaFile)
		if err != nil {
			continue
		}

		tableName := dbc.TableName(meta)

		// Find text columns to search
		var textCols []string
		for _, f := range meta.Fields {
			if f.Type == "string" {
				textCols = append(textCols, fmt.Sprintf("`%s`", f.Name))
			} else if f.Type == "Loc" {
				textCols = append(textCols, fmt.Sprintf("`%s_enus`", f.Name))
			}
		}

		if len(textCols) == 0 {
			continue
		}

		// Build WHERE clause: any text column LIKE '%pattern%'
		var conditions []string
		for _, col := range textCols {
			conditions = append(conditions, fmt.Sprintf("%s LIKE ?", col))
		}
		whereClause := strings.Join(conditions, " OR ")

		// Build args
		likePattern := "%" + pattern + "%"
		queryArgs := make([]interface{}, len(textCols))
		for i := range queryArgs {
			queryArgs[i] = likePattern
		}

		// Count matches first
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE %s", tableName, whereClause)
		var matchCount int
		if err := db.QueryRow(countQuery, queryArgs...).Scan(&matchCount); err != nil || matchCount == 0 {
			continue
		}

		// Fetch matching rows (limit to 10 per table)
		// Show id + text columns
		showCols := []string{"`id`"}
		showCols = append(showCols, textCols...)

		selectQuery := fmt.Sprintf("SELECT %s FROM `%s` WHERE %s LIMIT 10",
			strings.Join(showCols, ", "), tableName, whereClause)

		rows, err := db.Query(selectQuery, queryArgs...)
		if err != nil {
			continue
		}

		fmt.Printf("\n=== %s (%d matches) ===\n", tableName, matchCount)
		cols, _ := rows.Columns()
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}

		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				continue
			}
			var parts []string
			for i, v := range vals {
				switch val := v.(type) {
				case nil:
					continue
				case []byte:
					s := string(val)
					if s == "" {
						continue
					}
					parts = append(parts, fmt.Sprintf("%s=%s", cols[i], s))
				default:
					parts = append(parts, fmt.Sprintf("%s=%v", cols[i], val))
				}
			}
			fmt.Printf("  %s\n", strings.Join(parts, "  "))
		}
		rows.Close()

		if matchCount > 10 {
			fmt.Printf("  ... and %d more\n", matchCount-10)
		}
		totalMatches += matchCount
	}

	if totalMatches == 0 {
		fmt.Printf("No matches found for: %s\n", pattern)
		fmt.Println("Tip: use 'mithril mod dbc query' for more complex searches with SQL.")
	} else {
		fmt.Printf("\nTotal: %d matches\n", totalMatches)
	}

	return nil
}

func runModDBCInspect(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mithril mod dbc inspect <name>")
	}

	cfg := DefaultConfig()
	name := strings.TrimSuffix(args[0], ".dbc")
	tableName := strings.ToLower(name)

	meta, metaErr := dbc.GetMetaForDBC(name)

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

	// Query sample data from MySQL
	db, err := openDBCDB(cfg)
	if err != nil {
		fmt.Printf("  (Cannot connect to dbc database for sample data: %v)\n", err)
		return nil
	}
	defer db.Close()

	var recordCount int
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)).Scan(&recordCount); err != nil {
		fmt.Printf("  Table '%s' not found in dbc database.\n", tableName)
		return nil
	}

	fmt.Printf("Records: %d\n", recordCount)

	// Show first 5 rows
	rows, err := db.Query(fmt.Sprintf("SELECT * FROM `%s` LIMIT 5", tableName))
	if err != nil {
		return nil
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	showCols := 6
	if len(cols) < showCols {
		showCols = len(cols)
	}

	fmt.Printf("\nFirst rows (showing first %d of %d columns):\n\n", showCols, len(cols))

	for i := 0; i < showCols; i++ {
		fmt.Printf("%-20s", truncate(cols[i], 18))
	}
	fmt.Println()
	fmt.Println(strings.Repeat("-", showCols*20))

	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		for i := 0; i < showCols; i++ {
			s := "NULL"
			if vals[i] != nil {
				switch v := vals[i].(type) {
				case []byte:
					s = string(v)
				default:
					s = fmt.Sprintf("%v", v)
				}
			}
			fmt.Printf("%-20s", truncate(s, 18))
		}
		fmt.Println()
	}

	return nil
}

// --- Helper functions ---

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
