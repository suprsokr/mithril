package dbc

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ExportModifiedDBCs exports all DBC tables that have changed since import.
// Uses CHECKSUM TABLE to detect changes. Returns the list of exported table names.
func ExportModifiedDBCs(db *sql.DB, metaFiles []string, baselineDir, exportDir string) ([]string, error) {
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return nil, fmt.Errorf("create export dir: %w", err)
	}

	var exported []string
	for _, metaFile := range metaFiles {
		meta, err := LoadEmbeddedMeta(metaFile)
		if err != nil {
			continue
		}

		tableName := TableName(meta)

		// Check if table exists
		if !tableExistsCheck(db, tableName) {
			continue
		}

		// Compare current checksum against baseline (stored at import time)
		currentCS, err := GetTableChecksum(db, tableName)
		if err != nil {
			continue
		}
		baselineCS, err := GetStoredChecksum(db, tableName)
		if err != nil {
			continue
		}
		if currentCS == baselineCS {
			continue // table matches baseline — no modifications
		}

		// Export the table
		dbcFile, err := ExportTable(db, meta)
		if err != nil {
			fmt.Printf("    ⚠ Failed to export %s: %v\n", tableName, err)
			continue
		}

		outPath := filepath.Join(exportDir, meta.File)
		if err := WriteDBC(dbcFile, meta, outPath); err != nil {
			fmt.Printf("    ⚠ Failed to write %s: %v\n", meta.File, err)
			continue
		}

		exported = append(exported, tableName)
		fmt.Printf("    ✓ %s (SQL-exported, %d records)\n", tableName, dbcFile.Header.RecordCount)
	}

	return exported, nil
}

// ExportTable reads all rows from a DBC table and builds a DBCFile.
func ExportTable(db *sql.DB, meta *MetaFile) (*DBCFile, error) {
	tableName := TableName(meta)

	orderClause := buildOrderBy(meta.SortOrder)
	query := fmt.Sprintf("SELECT * FROM `%s`%s", tableName, orderClause)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query table %s: %w", tableName, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns for %s: %w", tableName, err)
	}

	dbcFile := &DBCFile{
		Header:      DBCHeader{Magic: [4]byte{'W', 'D', 'B', 'C'}},
		Records:     []Record{},
		StringBlock: []byte{0}, // first byte must be null
	}
	stringOffsets := map[string]uint32{"": 0}

	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row for %s: %w", tableName, err)
		}

		rec := make(Record)
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
				case "int32":
					rec[name] = toInt32(raw, cols, name)
				case "uint32":
					rec[name] = toUint32(raw, cols, name)
				case "uint8":
					rec[name] = toUint8(raw, cols, name)
				case "float":
					rec[name] = toFloat32(raw, cols, name)
				case "string":
					str := toString(raw, cols, name)
					rec[name] = getStringOffset(str, &dbcFile.StringBlock, stringOffsets)
				case "Loc":
					loc := make([]uint32, 17)
					for i := 0; i < 16; i++ {
						colName := fmt.Sprintf("%s_%s", name, strings.ToLower(LocLangs[i]))
						str := toString(raw, cols, colName)
						loc[i] = getStringOffset(str, &dbcFile.StringBlock, stringOffsets)
					}
					loc[16] = toUint32(raw, cols, fmt.Sprintf("%s_%s", name, strings.ToLower(LocLangs[16])))
					rec[name] = loc
				}
			}
		}
		dbcFile.Records = append(dbcFile.Records, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows for %s: %w", tableName, err)
	}

	dbcFile.Header.RecordCount = uint32(len(dbcFile.Records))
	dbcFile.Header.FieldCount = calculateFieldCount(meta)
	dbcFile.Header.RecordSize = calculateRecordSize(meta)
	dbcFile.Header.StringBlockSize = uint32(len(dbcFile.StringBlock))

	return dbcFile, nil
}

// --- Helpers ---

func buildOrderBy(sort []SortField) string {
	if len(sort) == 0 {
		return ""
	}
	parts := make([]string, len(sort))
	for i, sf := range sort {
		dir := strings.ToUpper(sf.Direction)
		if dir != "ASC" && dir != "DESC" {
			dir = "ASC"
		}
		parts[i] = fmt.Sprintf("`%s` %s", sf.Name, dir)
	}
	return " ORDER BY " + strings.Join(parts, ", ")
}

func getStringOffset(s string, block *[]byte, offsets map[string]uint32) uint32 {
	if off, ok := offsets[s]; ok {
		return off
	}
	off := uint32(len(*block))
	*block = append(*block, []byte(s)...)
	*block = append(*block, 0)
	offsets[s] = off
	return off
}

func tableExistsCheck(db *sql.DB, tableName string) bool {
	var exists string
	err := db.QueryRow(
		"SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		tableName,
	).Scan(&exists)
	return err == nil
}

// FieldCount returns the number of individual fields for a meta (exported for use by cmd layer).
func FieldCount(meta *MetaFile) uint32 {
	return calculateFieldCount(meta)
}

// calculateFieldCount counts the number of individual fields in the DBC header.
func calculateFieldCount(meta *MetaFile) uint32 {
	count := uint32(0)
	for _, f := range meta.Fields {
		repeat := uint32(f.Count)
		if repeat == 0 {
			repeat = 1
		}
		switch f.Type {
		case "int32", "uint32", "float", "string", "uint8":
			count += repeat
		case "Loc":
			count += 17 * repeat
		}
	}
	return count
}

// calculateRecordSize computes the byte size of one record from the meta.
func calculateRecordSize(meta *MetaFile) uint32 {
	size := uint32(0)
	for _, f := range meta.Fields {
		repeat := uint32(f.Count)
		if repeat == 0 {
			repeat = 1
		}
		switch f.Type {
		case "int32", "uint32", "float", "string":
			size += 4 * repeat
		case "uint8":
			size += 1 * repeat
		case "Loc":
			size += 4 * 17 * repeat
		}
	}
	return size
}

// --- Type conversion helpers (from sql.Rows.Scan output) ---

func toInt32(raw []interface{}, cols []string, name string) int32 {
	for i, col := range cols {
		if col == name && raw[i] != nil {
			switch v := raw[i].(type) {
			case int64:
				return int32(v)
			case []byte:
				if n, err := strconv.ParseInt(string(v), 10, 32); err == nil {
					return int32(n)
				}
			}
		}
	}
	return 0
}

func toUint32(raw []interface{}, cols []string, name string) uint32 {
	for i, col := range cols {
		if col == name && raw[i] != nil {
			switch v := raw[i].(type) {
			case int64:
				return uint32(v)
			case uint64:
				return uint32(v)
			case []byte:
				if n, err := strconv.ParseUint(string(v), 10, 32); err == nil {
					return uint32(n)
				}
				// Try float (DECIMAL columns)
				if f, err := strconv.ParseFloat(string(v), 64); err == nil {
					return uint32(f)
				}
			}
		}
	}
	return 0
}

func toUint8(raw []interface{}, cols []string, name string) uint8 {
	for i, col := range cols {
		if col == name && raw[i] != nil {
			switch v := raw[i].(type) {
			case int64:
				return uint8(v)
			case uint64:
				return uint8(v)
			case []byte:
				if n, err := strconv.ParseUint(string(v), 10, 8); err == nil {
					return uint8(n)
				}
			case string:
				if n, err := strconv.ParseUint(v, 10, 8); err == nil {
					return uint8(n)
				}
			}
		}
	}
	return 0
}

func toFloat32(raw []interface{}, cols []string, name string) float32 {
	for i, col := range cols {
		if col == name && raw[i] != nil {
			switch v := raw[i].(type) {
			case float64:
				return float32(v)
			case float32:
				return v
			case []byte:
				if f, err := strconv.ParseFloat(string(v), 64); err == nil {
					result := float32(f)
					if math.IsNaN(float64(result)) {
						return 0
					}
					return result
				}
			case string:
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return float32(f)
				}
			}
		}
	}
	return 0
}

func toString(raw []interface{}, cols []string, name string) string {
	for i, col := range cols {
		if col == name && raw[i] != nil {
			switch v := raw[i].(type) {
			case string:
				return v
			case []byte:
				return string(v)
			}
		}
	}
	return ""
}
