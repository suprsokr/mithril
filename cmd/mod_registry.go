package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	registryBaseURL = "https://raw.githubusercontent.com/suprsokr/mithril-registry/main"
	registryModsURL = registryBaseURL + "/mods"
	// GitHub API to list files in the mods/ directory
	registryAPIURL = "https://api.github.com/repos/suprsokr/mithril-registry/contents/mods"
)

// RegistryEntry represents a mod in the registry.
type RegistryEntry struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Repo        string            `json:"repo"`
	Tags        []string          `json:"tags"`
	Version     string            `json:"version"`
	ModTypes    []string          `json:"mod_types"`
	Releases    map[string]string `json:"releases,omitempty"`
}

func runModRegistry(subcmd string, args []string) error {
	switch subcmd {
	case "search":
		return runRegistrySearch(args)
	case "info":
		return runRegistryInfo(args)
	case "install":
		return runRegistryInstall(args)
	case "list":
		return runRegistryList(args)
	case "-h", "--help", "help":
		fmt.Print(registryUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod registry command: %s", subcmd)
	}
}

const registryUsage = `Mithril Mod Registry - Discover and install community mods

Usage:
  mithril mod registry <command> [args]

Commands:
  list                      List all mods in the registry
  search <query>            Search mods by name, description, or tags
  info <mod-name>           Show detailed info about a mod
  install <mod-name>        Clone a mod's source repo and set it up locally

Examples:
  mithril mod registry list
  mithril mod registry search "flying"
  mithril mod registry info fly-in-azeroth
  mithril mod registry install fly-in-azeroth
`

func runRegistryList(args []string) error {
	entries, err := fetchRegistryIndex()
	if err != nil {
		return fmt.Errorf("fetch registry: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No mods found in the registry.")
		return nil
	}

	fmt.Println("=== Mithril Mod Registry ===")
	fmt.Println()
	fmt.Printf("%-25s %-15s %-15s %s\n", "Name", "Author", "Types", "Description")
	fmt.Println(strings.Repeat("-", 85))

	for _, entry := range entries {
		types := strings.Join(entry.ModTypes, ",")
		desc := entry.Description
		if len(desc) > 35 {
			desc = desc[:32] + "..."
		}
		fmt.Printf("%-25s %-15s %-15s %s\n", entry.Name, entry.Author, types, desc)
	}

	fmt.Printf("\nTotal: %d mod(s)\n", len(entries))
	return nil
}

func runRegistrySearch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mithril mod registry search <query>")
	}
	query := strings.ToLower(args[0])

	entries, err := fetchRegistryIndex()
	if err != nil {
		return fmt.Errorf("fetch registry: %w", err)
	}

	var matches []RegistryEntry
	for _, entry := range entries {
		if matchesQuery(entry, query) {
			matches = append(matches, entry)
		}
	}

	if len(matches) == 0 {
		fmt.Printf("No mods found matching: %s\n", query)
		return nil
	}

	fmt.Printf("=== Search: %s (%d results) ===\n\n", query, len(matches))
	for _, entry := range matches {
		fmt.Printf("  %s — %s\n", entry.Name, entry.Description)
		fmt.Printf("    Author: %s  Tags: %s  Types: %s\n",
			entry.Author, strings.Join(entry.Tags, ", "), strings.Join(entry.ModTypes, ", "))
		if entry.Repo != "" {
			fmt.Printf("    Repo: %s\n", entry.Repo)
		}
		fmt.Println()
	}

	return nil
}

func runRegistryInfo(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mithril mod registry info <mod-name>")
	}
	name := args[0]

	entry, err := fetchRegistryEntry(name)
	if err != nil {
		return fmt.Errorf("fetch mod info: %w", err)
	}

	fmt.Printf("=== %s ===\n\n", entry.Name)
	fmt.Printf("  Description: %s\n", entry.Description)
	fmt.Printf("  Author:      %s\n", entry.Author)
	fmt.Printf("  Version:     %s\n", entry.Version)
	fmt.Printf("  Repo:        %s\n", entry.Repo)
	fmt.Printf("  Tags:        %s\n", strings.Join(entry.Tags, ", "))
	fmt.Printf("  Mod types:   %s\n", strings.Join(entry.ModTypes, ", "))

	if release, ok := entry.Releases["latest"]; ok {
		fmt.Printf("  Release:     %s\n", release)
		fmt.Println("               (pre-built artifacts for non-mithril users)")
	}

	fmt.Println()
	fmt.Printf("Install with: mithril mod registry install %s\n", entry.Name)

	return nil
}

func runRegistryInstall(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mithril mod registry install <mod-name>")
	}
	name := args[0]
	cfg := DefaultConfig()

	entry, err := fetchRegistryEntry(name)
	if err != nil {
		return fmt.Errorf("fetch mod info: %w", err)
	}

	modDir := cfg.ModDir(name)
	if _, err := os.Stat(modDir); err == nil {
		return fmt.Errorf("mod '%s' already exists at %s\nRemove it first to reinstall", name, modDir)
	}

	fmt.Printf("=== Installing: %s ===\n", entry.Name)
	fmt.Printf("  %s\n", entry.Description)
	fmt.Printf("  Author: %s\n\n", entry.Author)

	return installFromGit(cfg, entry)
}

// installFromGit clones the mod's repo into the modules directory.
func installFromGit(cfg *Config, entry RegistryEntry) error {
	if entry.Repo == "" {
		return fmt.Errorf("no repo URL for mod %s", entry.Name)
	}

	modDir := cfg.ModDir(entry.Name)
	fmt.Printf("Cloning %s...\n", entry.Repo)

	cmd := exec.Command("git", "clone", entry.Repo, modDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Check if mod.json exists, create one if not
	modJsonPath := filepath.Join(modDir, "mod.json")
	if _, err := os.Stat(modJsonPath); os.IsNotExist(err) {
		meta := ModMeta{
			Name:      entry.Name,
			CreatedAt: timeNow(),
		}
		data, _ := json.MarshalIndent(meta, "", "  ")
		os.WriteFile(modJsonPath, data, 0644)
	}

	fmt.Printf("\n✓ Installed %s to %s\n", entry.Name, modDir)
	printPostInstall(entry)
	return nil
}

func printPostInstall(entry RegistryEntry) {
	fmt.Println()
	fmt.Println("Next steps:")
	for _, t := range entry.ModTypes {
		switch t {
		case "dbc":
			fmt.Printf("  mithril mod build --mod %s       # Build DBC patches\n", entry.Name)
		case "addon":
			fmt.Printf("  mithril mod build --mod %s       # Build addon patches\n", entry.Name)
		case "sql":
			fmt.Printf("  mithril mod sql apply --mod %s   # Apply SQL migrations\n", entry.Name)
		case "core":
			fmt.Printf("  mithril mod core apply --mod %s  # Apply core patches\n", entry.Name)
		case "binary-patch":
			fmt.Printf("  mithril mod patch list           # See available binary patches\n")
		}
	}
}

// --- HTTP helpers ---

func fetchRegistryIndex() ([]RegistryEntry, error) {
	// Fetch the directory listing from GitHub API
	resp, err := http.Get(registryAPIURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse GitHub contents API response
	var files []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}

	var entries []RegistryEntry
	for _, f := range files {
		if !strings.HasSuffix(f.Name, ".json") {
			continue
		}

		entry, err := fetchJSON[RegistryEntry](f.DownloadURL)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

func fetchRegistryEntry(name string) (RegistryEntry, error) {
	url := registryModsURL + "/" + name + ".json"
	return fetchJSON[RegistryEntry](url)
}

func fetchJSON[T any](url string) (T, error) {
	var zero T
	resp, err := http.Get(url)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return zero, fmt.Errorf("not found")
	}
	if resp.StatusCode != 200 {
		return zero, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, err
	}
	return result, nil
}

func matchesQuery(entry RegistryEntry, query string) bool {
	if strings.Contains(strings.ToLower(entry.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Description), query) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Author), query) {
		return true
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	for _, t := range entry.ModTypes {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}
	return false
}

