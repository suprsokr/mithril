// Package patcher applies binary patches to the WoW client executable.
// Patches are described as JSON files with hex addresses and byte values.
package patcher

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Clean WoW 3.3.5a (12340) client MD5.
const CleanClientMD5 = "45892bdedd0ad70aed4ccd22d9fb5984"

// PatchFile represents a binary patch JSON file.
type PatchFile struct {
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description,omitempty"`
	Patches     []Patch `json:"patches"`
}

// Patch represents a single address+bytes replacement.
type Patch struct {
	Address string   `json:"address"`
	Bytes   []string `json:"bytes"`
}

// AppliedPatch tracks a patch that has been applied.
type AppliedPatch struct {
	Name      string `json:"name"`
	AppliedAt string `json:"applied_at"`
}

// Tracker records which patches have been applied.
type Tracker struct {
	Applied []AppliedPatch `json:"applied"`
}

// LoadPatchFile reads and parses a patch JSON file.
func LoadPatchFile(path string) (*PatchFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read patch file: %w", err)
	}
	var pf PatchFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse patch JSON: %w", err)
	}
	return &pf, nil
}

// ApplyPatchFile applies all patches in a PatchFile to an executable.
func ApplyPatchFile(exePath string, pf *PatchFile) error {
	data, err := os.ReadFile(exePath)
	if err != nil {
		return fmt.Errorf("read executable: %w", err)
	}

	for i, patch := range pf.Patches {
		addr, err := parseAddress(patch.Address)
		if err != nil {
			return fmt.Errorf("patch %d: invalid address %q: %w", i, patch.Address, err)
		}

		bytes, err := parseBytes(patch.Bytes)
		if err != nil {
			return fmt.Errorf("patch %d: invalid bytes: %w", i, err)
		}

		endAddr := addr + len(bytes)
		if endAddr > len(data) {
			return fmt.Errorf("patch %d: address 0x%x + %d bytes exceeds file size (%d)",
				i, addr, len(bytes), len(data))
		}

		copy(data[addr:endAddr], bytes)
	}

	if err := os.WriteFile(exePath, data, 0644); err != nil {
		return fmt.Errorf("write patched executable: %w", err)
	}

	return nil
}

// EnsureBackup creates a backup of the executable if one doesn't exist.
// Returns the backup path.
func EnsureBackup(exePath string) (string, error) {
	backupPath := exePath + ".clean"
	if _, err := os.Stat(backupPath); err == nil {
		return backupPath, nil // backup already exists
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		return "", fmt.Errorf("read executable for backup: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}

	return backupPath, nil
}

// VerifyCleanClient checks if a file matches the expected clean WoW 3.3.5a MD5.
func VerifyCleanClient(path string) (bool, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", err
	}
	hash := md5.Sum(data)
	actual := hex.EncodeToString(hash[:])
	return actual == CleanClientMD5, actual, nil
}

// LoadTracker loads the patch tracker from disk.
func LoadTracker(path string) (*Tracker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Tracker{}, nil
		}
		return nil, err
	}
	var t Tracker
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// SaveTracker writes the patch tracker to disk.
func SaveTracker(path string, t *Tracker) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// IsApplied checks if a patch name is already tracked as applied.
func (t *Tracker) IsApplied(name string) bool {
	for _, a := range t.Applied {
		if a.Name == name {
			return true
		}
	}
	return false
}

// MarkApplied records a patch as applied.
func (t *Tracker) MarkApplied(name, timestamp string) {
	t.Applied = append(t.Applied, AppliedPatch{
		Name:      name,
		AppliedAt: timestamp,
	})
}

// RestoreFromBackup restores the executable from its clean backup and clears the tracker.
func RestoreFromBackup(exePath string) error {
	backupPath := exePath + ".clean"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	if err := os.WriteFile(exePath, data, 0644); err != nil {
		return fmt.Errorf("write restored executable: %w", err)
	}
	return nil
}

func parseAddress(s string) (int, error) {
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	val, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, err
	}
	return int(val), nil
}

func parseBytes(hexBytes []string) ([]byte, error) {
	result := make([]byte, len(hexBytes))
	for i, s := range hexBytes {
		s = strings.TrimPrefix(s, "0x")
		s = strings.TrimPrefix(s, "0X")
		val, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			return nil, fmt.Errorf("byte %d (%q): %w", i, s, err)
		}
		result[i] = byte(val)
	}
	return result, nil
}
