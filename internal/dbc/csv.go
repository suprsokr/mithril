package dbc

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// ExportCSV writes a parsed DBC file to CSV format with named columns.
// Loc fields are expanded to individual locale columns (e.g., spell_name_enUS).
// String fields are resolved to their actual string values from the string block.
func ExportCSV(dbc *DBCFile, meta *MetaFile, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create CSV file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Write header row
	headers := ExpandedFieldNames(meta)
	if err := w.Write(headers); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	// Write each record
	for _, rec := range dbc.Records {
		row := make([]string, 0, len(headers))

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

				val, exists := rec[name]
				if !exists {
					if field.Type == "Loc" {
						// Loc expands to 17 columns
						for k := 0; k < 17; k++ {
							row = append(row, "")
						}
					} else {
						row = append(row, "")
					}
					continue
				}

				switch field.Type {
				case "int32":
					row = append(row, fmt.Sprintf("%d", val.(int32)))
				case "uint32":
					row = append(row, fmt.Sprintf("%d", val.(uint32)))
				case "uint8":
					row = append(row, fmt.Sprintf("%d", val.(uint8)))
				case "float":
					row = append(row, formatFloat(val.(float32)))
				case "string":
					offset := val.(uint32)
					str := ReadString(dbc.StringBlock, offset)
					row = append(row, str)
				case "Loc":
					loc := val.([]uint32)
					for i := 0; i < 17; i++ {
						if i < 16 {
							// Locale string slot
							str := ReadString(dbc.StringBlock, loc[i])
							row = append(row, str)
						} else {
							// Flags slot
							row = append(row, fmt.Sprintf("%d", loc[i]))
						}
					}
				}
			}
		}

		if err := w.Write(row); err != nil {
			return fmt.Errorf("write CSV record: %w", err)
		}
	}

	return nil
}

// ImportCSV reads a CSV file and reconstructs a DBCFile.
// String and Loc string values are written into a new string block.
func ImportCSV(csvPath string, meta *MetaFile) (*DBCFile, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("open CSV file: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true

	allRows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}

	if len(allRows) < 1 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	headerRow := allRows[0]
	dataRows := allRows[1:]

	// Build header → column index map
	colIndex := make(map[string]int)
	for i, h := range headerRow {
		colIndex[h] = i
	}

	// String block builder: offset 0 is always the empty string
	var stringBlock []byte
	stringBlock = append(stringBlock, 0) // null terminator at offset 0
	stringMap := map[string]uint32{"": 0}

	addString := func(s string) uint32 {
		if offset, ok := stringMap[s]; ok {
			return offset
		}
		offset := uint32(len(stringBlock))
		stringBlock = append(stringBlock, []byte(s)...)
		stringBlock = append(stringBlock, 0) // null terminator
		stringMap[s] = offset
		return offset
	}

	// Compute record size and field count from meta
	recordSize := uint32(0)
	fieldCount := uint32(0)
	for _, field := range meta.Fields {
		elemSize, err := sizeOf(field.Type)
		if err != nil {
			return nil, fmt.Errorf("unknown field type %s: %w", field.Type, err)
		}
		repeat := uint32(field.Count)
		if repeat == 0 {
			repeat = 1
		}
		recordSize += uint32(elemSize) * repeat
		// FieldCount in DBC header counts uint32-sized slots
		switch field.Type {
		case "Loc":
			fieldCount += 17 * repeat
		case "uint8", "int8":
			// These take 1 byte but the header field count is per-record-size/4
			// Actually DBC FieldCount = RecordSize / 4
			// So we just compute it at the end
		default:
			fieldCount += repeat
		}
	}
	fieldCount = recordSize / 4

	records := make([]Record, 0, len(dataRows))

	for rowIdx, row := range dataRows {
		rec := make(Record)
		colPos := 0 // current position in the expanded column list

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
					val, err := getCSVCell(row, colPos)
					if err != nil {
						return nil, fmt.Errorf("row %d field %s: %w", rowIdx+1, name, err)
					}
					n, err := strconv.ParseInt(val, 10, 32)
					if err != nil {
						// Try parsing as 0 for empty cells
						if val == "" {
							n = 0
						} else {
							return nil, fmt.Errorf("row %d field %s: parse int32 %q: %w", rowIdx+1, name, val, err)
						}
					}
					rec[name] = int32(n)
					colPos++

				case "uint32":
					val, err := getCSVCell(row, colPos)
					if err != nil {
						return nil, fmt.Errorf("row %d field %s: %w", rowIdx+1, name, err)
					}
					n, err := strconv.ParseUint(val, 10, 32)
					if err != nil {
						if val == "" {
							n = 0
						} else {
							return nil, fmt.Errorf("row %d field %s: parse uint32 %q: %w", rowIdx+1, name, val, err)
						}
					}
					rec[name] = uint32(n)
					colPos++

				case "uint8":
					val, err := getCSVCell(row, colPos)
					if err != nil {
						return nil, fmt.Errorf("row %d field %s: %w", rowIdx+1, name, err)
					}
					n, err := strconv.ParseUint(val, 10, 8)
					if err != nil {
						if val == "" {
							n = 0
						} else {
							return nil, fmt.Errorf("row %d field %s: parse uint8 %q: %w", rowIdx+1, name, val, err)
						}
					}
					rec[name] = uint8(n)
					colPos++

				case "float":
					val, err := getCSVCell(row, colPos)
					if err != nil {
						return nil, fmt.Errorf("row %d field %s: %w", rowIdx+1, name, err)
					}
					f, err := strconv.ParseFloat(val, 32)
					if err != nil {
						if val == "" {
							f = 0
						} else {
							return nil, fmt.Errorf("row %d field %s: parse float %q: %w", rowIdx+1, name, val, err)
						}
					}
					rec[name] = float32(f)
					colPos++

				case "string":
					val, _ := getCSVCell(row, colPos)
					offset := addString(val)
					rec[name] = offset
					colPos++

				case "Loc":
					loc := make([]uint32, 17)
					for i := 0; i < 17; i++ {
						val, _ := getCSVCell(row, colPos)
						if i < 16 {
							// Locale string slot
							loc[i] = addString(val)
						} else {
							// Flags slot
							n, _ := strconv.ParseUint(val, 10, 32)
							loc[i] = uint32(n)
						}
						colPos++
					}
					rec[name] = loc
				}
			}
		}

		records = append(records, rec)
	}

	header := DBCHeader{
		Magic:           [4]byte{'W', 'D', 'B', 'C'},
		RecordCount:     uint32(len(records)),
		FieldCount:      fieldCount,
		RecordSize:      recordSize,
		StringBlockSize: uint32(len(stringBlock)),
	}

	return &DBCFile{
		Header:      header,
		Records:     records,
		StringBlock: stringBlock,
	}, nil
}

// getCSVCell safely gets a cell value from a row.
func getCSVCell(row []string, idx int) (string, error) {
	if idx >= len(row) {
		return "", nil
	}
	return strings.TrimSpace(row[idx]), nil
}

// formatFloat formats a float32 for CSV output, avoiding unnecessary trailing zeros.
func formatFloat(f float32) string {
	if f == 0 {
		return "0"
	}
	if math.IsNaN(float64(f)) {
		return "0"
	}
	// Use enough precision to round-trip
	s := strconv.FormatFloat(float64(f), 'f', -1, 32)
	return s
}
