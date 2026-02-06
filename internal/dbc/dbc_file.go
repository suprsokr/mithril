// Package dbc provides parsing and writing of WoW 3.3.5a DBC (DataBase Client) files.
// Field schemas are embedded from meta/*.meta.json files with known column names.
//
// Adapted from thorium-cli's DBC package.

package dbc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// DBCHeader is the 20-byte header at the start of every DBC file.
type DBCHeader struct {
	Magic           [4]byte
	RecordCount     uint32
	FieldCount      uint32
	RecordSize      uint32
	StringBlockSize uint32
}

// SortField describes a sort key used in meta files.
type SortField struct {
	Name      string `json:"name"`
	Direction string `json:"direction"` // "ASC" or "DESC"
}

// FieldMeta describes a single logical field in a DBC record.
type FieldMeta struct {
	Name  string `json:"name"`
	Type  string `json:"type"` // int32, uint32, float, string, Loc
	Count uint32 `json:"count,omitempty"`
}

// MetaFile is the schema description for a DBC file.
type MetaFile struct {
	File        string      `json:"file"`
	TableName   string      `json:"tableName,omitempty"`
	PrimaryKeys []string    `json:"primaryKeys"`
	UniqueKeys  [][]string  `json:"uniqueKeys,omitempty"`
	SortOrder   []SortField `json:"sortOrder,omitempty"`
	Fields      []FieldMeta `json:"fields"`
}

// Record is a single DBC record stored as field-name → value.
type Record map[string]interface{}

// DBCFile is an in-memory representation of a parsed DBC file.
type DBCFile struct {
	Header      DBCHeader
	Records     []Record
	StringBlock []byte
}

// LocLangs are the 16 locale slots + 1 flags slot in a Loc field.
var LocLangs = []string{
	"enUS", "koKR", "frFR", "deDE",
	"enCN", "enTW", "esES", "esMX",
	"ruRU", "jaJP", "ptPT", "itIT",
	"unknown1", "unknown2", "unknown3", "unknown4",
	"flags",
}

// LoadMeta reads and parses a meta JSON file from disk.
func LoadMeta(path string) (MetaFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MetaFile{}, fmt.Errorf("failed to read meta file %s: %w", path, err)
	}
	var meta MetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return MetaFile{}, fmt.Errorf("failed to parse meta JSON %s: %w", path, err)
	}
	return meta, nil
}

// LoadDBC reads a DBC binary file and parses it using the given schema.
func LoadDBC(dbcPath string, meta MetaFile) (DBCFile, error) {
	data, err := os.ReadFile(dbcPath)
	if err != nil {
		return DBCFile{}, fmt.Errorf("failed to read DBC file %s: %w", dbcPath, err)
	}
	return LoadDBCFromBytes(data, meta)
}

// LoadDBCFromBytes parses DBC binary data using the given schema.
func LoadDBCFromBytes(data []byte, meta MetaFile) (DBCFile, error) {
	if len(data) < 20 {
		return DBCFile{}, fmt.Errorf("data too small to be a valid DBC (%d bytes)", len(data))
	}

	header, err := ParseHeader(data[:20])
	if err != nil {
		return DBCFile{}, err
	}

	recordsStart := 20
	stringBlockStart := recordsStart + int(header.RecordCount*header.RecordSize)
	if stringBlockStart+int(header.StringBlockSize) > len(data) {
		return DBCFile{}, fmt.Errorf("data too small for records + string block")
	}
	stringBlock := data[stringBlockStart : stringBlockStart+int(header.StringBlockSize)]

	records, err := ParseRecords(data, recordsStart, header, meta, stringBlock)
	if err != nil {
		return DBCFile{}, err
	}

	return DBCFile{
		Header:      header,
		Records:     records,
		StringBlock: stringBlock,
	}, nil
}

// ParseHeader parses the 20-byte DBC header.
func ParseHeader(data []byte) (DBCHeader, error) {
	header := DBCHeader{
		Magic:           [4]byte{data[0], data[1], data[2], data[3]},
		RecordCount:     binary.LittleEndian.Uint32(data[4:8]),
		FieldCount:      binary.LittleEndian.Uint32(data[8:12]),
		RecordSize:      binary.LittleEndian.Uint32(data[12:16]),
		StringBlockSize: binary.LittleEndian.Uint32(data[16:20]),
	}
	if string(header.Magic[:]) != "WDBC" {
		return DBCHeader{}, fmt.Errorf("invalid DBC file magic: %s", string(header.Magic[:]))
	}
	return header, nil
}

// sizeOf returns the byte size of a single element of the given field type.
func sizeOf(typ string) (int, error) {
	switch typ {
	case "int32", "uint32", "float", "string":
		return 4, nil
	case "uint8", "int8":
		return 1, nil
	case "Loc":
		return 17 * 4, nil
	default:
		return 0, fmt.Errorf("unknown field type: %s", typ)
	}
}

// ParseRecords reads all records from raw DBC data.
func ParseRecords(data []byte, start int, header DBCHeader, meta MetaFile, stringBlock []byte) ([]Record, error) {
	// Compute expected record size from meta.
	expectedRecordSize := 0
	for _, field := range meta.Fields {
		elemSize, err := sizeOf(field.Type)
		if err != nil {
			return nil, err
		}
		repeat := int(field.Count)
		if repeat == 0 {
			repeat = 1
		}
		expectedRecordSize += elemSize * repeat
	}

	if uint32(expectedRecordSize) != header.RecordSize {
		return nil, fmt.Errorf("record size mismatch: header.RecordSize=%d but meta expects %d",
			header.RecordSize, expectedRecordSize)
	}

	totalRecordsBytes := int(header.RecordCount) * int(header.RecordSize)
	if start+totalRecordsBytes > len(data) {
		return nil, fmt.Errorf("data too small for all records: need %d bytes at offset %d, have %d",
			totalRecordsBytes, start, len(data))
	}

	records := make([]Record, 0, header.RecordCount)
	for i := uint32(0); i < header.RecordCount; i++ {
		rec := make(Record)
		recordOffset := start + int(i*header.RecordSize)
		offset := 0

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

				elemSize, _ := sizeOf(field.Type)
				if recordOffset+offset+elemSize > len(data) {
					return nil, fmt.Errorf("out of bounds reading record %d field %s", i, name)
				}

				switch field.Type {
				case "int32":
					val := int32(binary.LittleEndian.Uint32(data[recordOffset+offset : recordOffset+offset+4]))
					rec[name] = val
					offset += 4
				case "uint32":
					val := binary.LittleEndian.Uint32(data[recordOffset+offset : recordOffset+offset+4])
					rec[name] = val
					offset += 4
				case "uint8":
					rec[name] = data[recordOffset+offset]
					offset += 1
				case "float":
					bits := binary.LittleEndian.Uint32(data[recordOffset+offset : recordOffset+offset+4])
					rec[name] = math.Float32frombits(bits)
					offset += 4
				case "string":
					strOffset := binary.LittleEndian.Uint32(data[recordOffset+offset : recordOffset+offset+4])
					rec[name] = strOffset
					offset += 4
				case "Loc":
					loc := make([]uint32, 17)
					for col := 0; col < 17; col++ {
						loc[col] = binary.LittleEndian.Uint32(data[recordOffset+offset : recordOffset+offset+4])
						offset += 4
					}
					rec[name] = loc
				default:
					return nil, fmt.Errorf("unknown field type: %s", field.Type)
				}
			}
		}

		if offset != expectedRecordSize {
			return nil, fmt.Errorf("parsed record %d consumed %d bytes but expected %d", i, offset, expectedRecordSize)
		}
		records = append(records, rec)
	}

	return records, nil
}

// ReadString extracts a null-terminated string from the string block.
func ReadString(stringBlock []byte, offset uint32) string {
	if offset >= uint32(len(stringBlock)) {
		return ""
	}
	end := offset
	for end < uint32(len(stringBlock)) && stringBlock[end] != 0 {
		end++
	}
	return string(stringBlock[offset:end])
}

// WriteDBC writes a DBCFile back to binary format.
func WriteDBC(dbc *DBCFile, meta *MetaFile, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Write header
	headerBuf := make([]byte, 20)
	copy(headerBuf[0:4], dbc.Header.Magic[:])
	binary.LittleEndian.PutUint32(headerBuf[4:8], dbc.Header.RecordCount)
	binary.LittleEndian.PutUint32(headerBuf[8:12], dbc.Header.FieldCount)
	binary.LittleEndian.PutUint32(headerBuf[12:16], dbc.Header.RecordSize)
	binary.LittleEndian.PutUint32(headerBuf[16:20], dbc.Header.StringBlockSize)
	if _, err := outFile.Write(headerBuf); err != nil {
		return err
	}

	// Write records
	recordData := make([]byte, dbc.Header.RecordCount*dbc.Header.RecordSize)
	offset := 0

	for _, rec := range dbc.Records {
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
					binary.LittleEndian.PutUint32(recordData[offset:offset+4], uint32(rec[name].(int32)))
					offset += 4
				case "uint32":
					binary.LittleEndian.PutUint32(recordData[offset:offset+4], rec[name].(uint32))
					offset += 4
				case "uint8":
					recordData[offset] = rec[name].(uint8)
					offset += 1
				case "float":
					bits := math.Float32bits(rec[name].(float32))
					binary.LittleEndian.PutUint32(recordData[offset:offset+4], bits)
					offset += 4
				case "string":
					binary.LittleEndian.PutUint32(recordData[offset:offset+4], rec[name].(uint32))
					offset += 4
				case "Loc":
					loc := rec[name].([]uint32)
					for _, v := range loc {
						binary.LittleEndian.PutUint32(recordData[offset:offset+4], v)
						offset += 4
					}
				}
			}
		}
	}

	if _, err := outFile.Write(recordData); err != nil {
		return err
	}

	// Write string block
	if _, err := outFile.Write(dbc.StringBlock); err != nil {
		return err
	}

	return nil
}

// ExpandedFieldNames returns the flat list of column names for a meta,
// expanding arrays (name_1, name_2, ...) and Loc fields (name_enUS, name_koKR, ...).
func ExpandedFieldNames(meta *MetaFile) []string {
	var names []string
	for _, field := range meta.Fields {
		repeat := int(field.Count)
		if repeat == 0 {
			repeat = 1
		}

		if field.Type == "Loc" {
			for j := 0; j < repeat; j++ {
				baseName := field.Name
				if field.Count > 1 {
					baseName = fmt.Sprintf("%s_%d", field.Name, j+1)
				}
				for _, lang := range LocLangs {
					names = append(names, fmt.Sprintf("%s_%s", baseName, lang))
				}
			}
		} else {
			for j := 0; j < repeat; j++ {
				name := field.Name
				if field.Count > 1 {
					name = fmt.Sprintf("%s_%d", field.Name, j+1)
				}
				names = append(names, name)
			}
		}
	}
	return names
}
