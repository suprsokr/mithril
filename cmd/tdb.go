package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ghRelease / ghAsset represent the subset of the GitHub Releases API we need.
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// downloadTDB fetches the latest TDB 335 full-world archive from the
// TrinityCore GitHub releases, extracts it, and places the SQL file into
// mithril-data/tdb/.
func downloadTDB(cfg *Config) error {
	tdbDir := filepath.Join(cfg.MithrilDir, "tdb")
	if err := os.MkdirAll(tdbDir, 0755); err != nil {
		return err
	}

	// Already have one?
	entries, _ := os.ReadDir(tdbDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "TDB_full_world_335") && strings.HasSuffix(e.Name(), ".sql") {
			printInfo("TDB file already exists: " + e.Name())
			return nil
		}
	}

	printInfo("Fetching latest TDB release from GitHub...")

	resp, err := http.Get("https://api.github.com/repos/TrinityCore/TrinityCore/releases?per_page=50")
	if err != nil {
		return fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var releases []ghRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return fmt.Errorf("parsing releases: %w", err)
	}

	asset, tag := findTDB335Asset(releases)
	if asset == nil {
		return fmt.Errorf(
			"no TDB 335 release found; download manually from https://github.com/TrinityCore/TrinityCore/releases")
	}

	printInfo(fmt.Sprintf("Downloading %s (release %s)...", asset.Name, tag))

	archivePath := filepath.Join(tdbDir, asset.Name)
	if err := downloadFile(archivePath, asset.BrowserDownloadURL); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	printInfo("Extracting TDB archive...")
	if err := runCmdDir(tdbDir, "7z", "x", "-y", archivePath); err != nil {
		if err2 := runCmdDir(tdbDir, "7za", "x", "-y", archivePath); err2 != nil {
			return fmt.Errorf("extraction failed (install p7zip): %w", err)
		}
	}

	os.Remove(archivePath)
	printSuccess("TDB database downloaded and extracted")
	return nil
}

// findTDB335Asset walks the releases list and returns the first matching
// TDB_full_world_335*.7z asset (newest first).
func findTDB335Asset(releases []ghRelease) (*ghAsset, string) {
	pat := regexp.MustCompile(`TDB_full_world_335.*\.7z`)

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].TagName > releases[j].TagName
	})

	for _, rel := range releases {
		if !strings.Contains(rel.TagName, "335") &&
			!strings.Contains(strings.ToLower(rel.TagName), "tdb") {
			continue
		}
		for i, a := range rel.Assets {
			if pat.MatchString(a.Name) {
				return &rel.Assets[i], rel.TagName
			}
		}
	}
	return nil, ""
}
