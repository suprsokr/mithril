package dbc

import (
	"embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

//go:embed meta/*.meta.json
var embeddedMeta embed.FS

// GetEmbeddedMetaFiles returns a list of all embedded meta file names.
func GetEmbeddedMetaFiles() ([]string, error) {
	entries, err := embeddedMeta.ReadDir("meta")
	if err != nil {
		return nil, fmt.Errorf("read embedded meta dir: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".meta.json") {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// LoadEmbeddedMeta loads a meta file from the embedded FS by name.
func LoadEmbeddedMeta(name string) (*MetaFile, error) {
	if !strings.HasSuffix(name, ".meta.json") {
		name = name + ".meta.json"
	}

	data, err := embeddedMeta.ReadFile(filepath.Join("meta", name))
	if err != nil {
		return nil, fmt.Errorf("read embedded meta %s: %w", name, err)
	}

	var meta MetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse meta %s: %w", name, err)
	}

	return &meta, nil
}

// GetMetaForDBC returns the meta file for a given DBC filename (e.g., "Spell" or "Spell.dbc").
func GetMetaForDBC(dbcName string) (*MetaFile, error) {
	// Strip .dbc extension if present
	name := strings.TrimSuffix(dbcName, ".dbc")
	name = strings.TrimSuffix(name, ".DBC")

	// Try direct lowercase match
	meta, err := LoadEmbeddedMeta(strings.ToLower(name))
	if err == nil {
		return meta, nil
	}

	// Scan all metas to find matching file
	files, err2 := GetEmbeddedMetaFiles()
	if err2 != nil {
		return nil, fmt.Errorf("no meta found for %s: %w (scan error: %w)", dbcName, err, err2)
	}

	for _, file := range files {
		m, err := LoadEmbeddedMeta(file)
		if err != nil {
			continue
		}
		// Match by file field (e.g., "Spell.dbc")
		mBase := strings.TrimSuffix(m.File, ".dbc")
		if strings.EqualFold(mBase, name) {
			return m, nil
		}
		// Match by base filename
		baseName := strings.TrimSuffix(file, ".meta.json")
		if strings.EqualFold(baseName, name) {
			return m, nil
		}
	}

	return nil, fmt.Errorf("no meta found for DBC: %s", dbcName)
}
