package dbc

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ImportAllDBCs imports all baseline DBC files that have known schemas into MySQL.
func ImportAllDBCs(db *sql.DB, dbcDir string, force bool) (int, int, error) {
	metaFiles, err := GetEmbeddedMetaFiles()
	if err != nil {
		return 0, 0, fmt.Errorf("get embedded meta files: %w", err)
	}

	imported := 0
	skipped := 0
	for _, metaFile := range metaFiles {
		meta, err := LoadEmbeddedMeta(metaFile)
		if err != nil {
			skipped++
			continue
		}

		dbcPath := findDBCFile(dbcDir, meta.File)
		if dbcPath == "" {
			skipped++
			continue
		}

		didImport, err := ImportDBC(db, dbcPath, meta, force)
		if err != nil {
			fmt.Printf("  ⚠ %s: %v\n", meta.File, err)
			skipped++
			continue
		}

		if didImport {
			imported++
		} else {
			skipped++
		}
	}

	return imported, skipped, nil
}

// ImportDBC imports a single DBC file into the MySQL dbc database.
// Returns true if the table was imported, false if skipped.
func ImportDBC(db *sql.DB, dbcPath string, meta *MetaFile, force bool) (bool, error) {
	if err := ensureChecksumTable(db); err != nil {
		return false, fmt.Errorf("ensure checksum table: %w", err)
	}

	tableName := TableName(meta)

	if err := ensureChecksumEntry(db, tableName); err != nil {
		return false, fmt.Errorf("ensure checksum entry for %s: %w", tableName, err)
	}

	if tableExists(db, force, tableName) {
		return false, nil
	}

	fmt.Printf("  Importing %-30s → %s ... ", meta.File, tableName)

	dbcFile, err := LoadDBC(dbcPath, *meta)
	if err != nil {
		fmt.Println("⚠")
		return false, fmt.Errorf("load DBC %s: %w", dbcPath, err)
	}

	checkUniqueKeys(dbcFile.Records, meta, tableName)

	if err := createTable(db, tableName, meta); err != nil {
		fmt.Println("⚠")
		return false, fmt.Errorf("create table %s: %w", tableName, err)
	}

	if err := insertRecords(db, tableName, &dbcFile, meta); err != nil {
		fmt.Println("⚠")
		return false, fmt.Errorf("insert records for %s: %w", tableName, err)
	}

	// Store the baseline checksum so exports can detect changes.
	// This value is never updated — it represents the pristine imported state.
	cs, err := GetTableChecksum(db, tableName)
	if err == nil {
		UpdateChecksum(db, tableName, cs)
	}

	fmt.Printf("✓ (%d records)\n", len(dbcFile.Records))
	return true, nil
}

// --- Table management ---

func tableExists(db *sql.DB, force bool, table string) bool {
	var exists string
	err := db.QueryRow(
		"SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		table,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		return false
	}
	if force {
		db.Exec("DROP TABLE IF EXISTS `" + table + "`")
		return false
	}
	return true
}

func createTable(db *sql.DB, tableName string, meta *MetaFile) error {
	var columns []string
	validFields := make(map[string]struct{})

	for _, field := range meta.Fields {
		repeat := int(field.Count)
		if repeat == 0 {
			repeat = 1
		}

		for j := 0; j < repeat; j++ {
			colName := field.Name
			if field.Count > 1 {
				colName = fmt.Sprintf("%s_%d", field.Name, j+1)
			}

			switch field.Type {
			case "int32":
				columns = append(columns, fmt.Sprintf("`%s` INT", colName))
			case "uint32":
				columns = append(columns, fmt.Sprintf("`%s` INT UNSIGNED", colName))
			case "uint8":
				columns = append(columns, fmt.Sprintf("`%s` TINYINT UNSIGNED", colName))
			case "float":
				columns = append(columns, fmt.Sprintf("`%s` DECIMAL(38,16)", colName))
			case "string":
				columns = append(columns, fmt.Sprintf("`%s` TEXT", colName))
			case "Loc":
				for i, lang := range LocLangs {
					locCol := fmt.Sprintf("%s_%s", colName, strings.ToLower(lang))
					if i == len(LocLangs)-1 {
						columns = append(columns, fmt.Sprintf("`%s` INT UNSIGNED", locCol))
					} else {
						columns = append(columns, fmt.Sprintf("`%s` TEXT", locCol))
					}
				}
			default:
				return fmt.Errorf("unknown field type: %s", field.Type)
			}

			validFields[colName] = struct{}{}
		}
	}

	// Primary key
	pkCols := []string{"`auto_id`"}
	if len(meta.PrimaryKeys) > 0 {
		var validPKs []string
		for _, pkc := range meta.PrimaryKeys {
			if _, ok := validFields[pkc]; ok {
				validPKs = append(validPKs, fmt.Sprintf("`%s`", pkc))
			}
		}
		if len(validPKs) > 0 {
			pkCols = validPKs
		} else {
			columns = append([]string{"`auto_id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT"}, columns...)
			pkCols = []string{"`auto_id`"}
		}
	}

	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS `%s` (%s, PRIMARY KEY(%s)",
		tableName, strings.Join(columns, ", "), strings.Join(pkCols, ", "),
	)

	// Unique keys
	for i, uk := range meta.UniqueKeys {
		if len(uk) == 0 {
			continue
		}
		cols := make([]string, len(uk))
		for j, c := range uk {
			cols[j] = fmt.Sprintf("`%s`", c)
		}
		query += fmt.Sprintf(", UNIQUE KEY `uk_%d` (%s)", i, strings.Join(cols, ", "))
	}

	query += ")"

	_, err := db.Exec(query)
	return err
}

// --- Record insertion ---

func insertRecords(db *sql.DB, tableName string, dbcFile *DBCFile, meta *MetaFile) error {
	total := len(dbcFile.Records)
	if total == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Build column list
	var columnsBase []string
	for _, field := range meta.Fields {
		repeat := int(field.Count)
		if repeat == 0 {
			repeat = 1
		}
		for j := 0; j < repeat; j++ {
			colName := field.Name
			if field.Count > 1 {
				colName = fmt.Sprintf("%s_%d", field.Name, j+1)
			}
			switch field.Type {
			case "int32", "uint32", "uint8", "float", "string":
				columnsBase = append(columnsBase, fmt.Sprintf("`%s`", colName))
			case "Loc":
				for _, lang := range LocLangs {
					columnsBase = append(columnsBase, fmt.Sprintf("`%s_%s`", colName, strings.ToLower(lang)))
				}
			}
		}
	}

	// Batch size: stay under MySQL's 65535 placeholder limit
	colsPerRow := len(columnsBase)
	maxPlaceholders := 60000
	batchSize := maxPlaceholders / colsPerRow
	if batchSize > 2000 {
		batchSize = 2000
	}

	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		records := dbcFile.Records[start:end]

		var allPlaceholders []string
		var allValues []interface{}

		for _, rec := range records {
			var rowPlaceholders []string
			for _, field := range meta.Fields {
				repeat := int(field.Count)
				if repeat == 0 {
					repeat = 1
				}
				for j := 0; j < repeat; j++ {
					name := field.Name
					if field.Count > 1 {
						name = fmt.Sprintf("%s_%d", field.Name, j+1)
					}
					switch field.Type {
					case "int32", "uint32", "uint8", "float":
						rowPlaceholders = append(rowPlaceholders, "?")
						allValues = append(allValues, rec[name])
					case "string":
						rowPlaceholders = append(rowPlaceholders, "?")
						offset := rec[name].(uint32)
						allValues = append(allValues, ReadString(dbcFile.StringBlock, offset))
					case "Loc":
						locArr := rec[name].([]uint32)
						numTexts := len(locArr) - 1
						for i := range LocLangs {
							rowPlaceholders = append(rowPlaceholders, "?")
							if i < numTexts {
								allValues = append(allValues, ReadString(dbcFile.StringBlock, locArr[i]))
							} else if i == numTexts {
								allValues = append(allValues, locArr[numTexts]) // flags
							} else {
								allValues = append(allValues, nil)
							}
						}
					}
				}
			}
			allPlaceholders = append(allPlaceholders, "("+strings.Join(rowPlaceholders, ", ")+")")
		}

		query := fmt.Sprintf(
			"INSERT INTO `%s` (%s) VALUES %s ON DUPLICATE KEY UPDATE %s",
			tableName,
			strings.Join(columnsBase, ", "),
			strings.Join(allPlaceholders, ", "),
			generateUpdateAssignments(columnsBase),
		)

		if _, err := tx.Exec(query, allValues...); err != nil {
			return fmt.Errorf("batch insert failed (%d–%d): %v", start, end, err)
		}
	}

	return tx.Commit()
}

func generateUpdateAssignments(columns []string) string {
	assignments := make([]string, len(columns))
	for i, col := range columns {
		assignments[i] = fmt.Sprintf("%s=VALUES(%s)", col, col)
	}
	return strings.Join(assignments, ", ")
}

// --- Checksum tracking ---

func ensureChecksumTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS dbc_checksum (
			table_name VARCHAR(255) NOT NULL PRIMARY KEY,
			checksum BIGINT UNSIGNED NOT NULL DEFAULT 0
		)`)
	// Migrate: drop baseline_checksum column if present (no longer used)
	if err == nil {
		db.Exec("ALTER TABLE dbc_checksum DROP COLUMN baseline_checksum")
	}
	return err
}

func ensureChecksumEntry(db *sql.DB, tableName string) error {
	var exists int
	err := db.QueryRow("SELECT 1 FROM dbc_checksum WHERE table_name = ?", tableName).Scan(&exists)
	if err == sql.ErrNoRows {
		_, insErr := db.Exec("INSERT INTO dbc_checksum (table_name, checksum) VALUES (?, 0)", tableName)
		return insErr
	}
	return err
}

// GetTableChecksum returns the CHECKSUM TABLE value for change detection.
func GetTableChecksum(db *sql.DB, tableName string) (uint64, error) {
	var tbl string
	var checksum sql.NullInt64
	err := db.QueryRow("CHECKSUM TABLE `" + tableName + "`").Scan(&tbl, &checksum)
	if err != nil {
		return 0, err
	}
	if !checksum.Valid {
		return 0, nil
	}
	return uint64(checksum.Int64), nil
}

// GetStoredChecksum retrieves the stored checksum from dbc_checksum.
func GetStoredChecksum(db *sql.DB, tableName string) (uint64, error) {
	var cs sql.NullInt64
	err := db.QueryRow("SELECT checksum FROM dbc_checksum WHERE table_name = ?", tableName).Scan(&cs)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !cs.Valid {
		return 0, nil
	}
	return uint64(cs.Int64), nil
}

// UpdateChecksum updates the stored checksum for a table.
func UpdateChecksum(db *sql.DB, tableName string, checksum uint64) error {
	_, err := db.Exec("UPDATE dbc_checksum SET checksum = ? WHERE table_name = ?", checksum, tableName)
	return err
}


// --- Unique key validation ---

func checkUniqueKeys(records []Record, meta *MetaFile, tableName string) {
	for i, uk := range meta.UniqueKeys {
		if len(uk) == 0 {
			continue
		}
		seen := map[string][]int{}
		for idx, rec := range records {
			var keyParts []string
			for _, col := range uk {
				val, ok := rec[col]
				if !ok {
					val = "<MISSING>"
				}
				keyParts = append(keyParts, fmt.Sprintf("%v", val))
			}
			keyStr := strings.Join(keyParts, ":")
			seen[keyStr] = append(seen[keyStr], idx)
		}
		for _, indices := range seen {
			if len(indices) > 1 {
				fmt.Printf("\n  Warning: duplicate records in '%s' for unique key #%d (%v):\n", tableName, i, uk)
				for _, idx := range indices {
					rec := records[idx]
					keys := make([]string, 0, len(rec))
					for k := range rec {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					fmt.Printf("    Record %d: {", idx)
					for ki, k := range keys {
						if ki > 0 {
							fmt.Print(", ")
						}
						fmt.Printf("%s: %v", k, rec[k])
					}
					fmt.Println("}")
				}
			}
		}
	}
}

// --- File finding ---

// findDBCFile searches for a DBC file case-insensitively.
func findDBCFile(dir, filename string) string {
	exact := filepath.Join(dir, filename)
	if _, err := os.Stat(exact); err == nil {
		return exact
	}

	lower := filepath.Join(dir, strings.ToLower(filename))
	if _, err := os.Stat(lower); err == nil {
		return lower
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	nameLower := strings.ToLower(filename)
	for _, entry := range entries {
		if strings.ToLower(entry.Name()) == nameLower {
			return filepath.Join(dir, entry.Name())
		}
	}
	return ""
}

// escapeSQLString escapes a string for use in a SQL statement.
func escapeSQLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\x00", "")
	return "'" + s + "'"
}

// Silence unused import warning for log package
var _ = log.Println
