package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func runModAddon(subcmd string, args []string) error {
	switch subcmd {
	case "create":
		return runModAddonCreate(args)
	case "list":
		return runModAddonList(args)
	case "search":
		return runModAddonSearch(args)
	case "edit":
		return runModAddonEdit(args)
	case "remove":
		return runModAddonRemove(args)
	case "-h", "--help", "help":
		fmt.Print(modUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod addon command: %s", subcmd)
	}
}

// runModAddonCreate copies a baseline addon file into a mod for editing.
func runModAddonCreate(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod addon create <path> --mod <mod_name>\n\nExample: mithril mod addon create Interface/FrameXML/SpellBookFrame.lua --mod my-mod")
	}

	cfg := DefaultConfig()
	addonPath := filepath.ToSlash(remaining[0])

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Check if already exists in mod
	modFilePath := filepath.Join(cfg.ModAddonsDir(modName), addonPath)
	if _, err := os.Stat(modFilePath); err == nil {
		return fmt.Errorf("addon file already exists in mod: %s", modFilePath)
	}

	// Copy from baseline
	if err := copyBaselineAddonToMod(cfg, modName, addonPath); err != nil {
		return err
	}

	fmt.Printf("✓ Copied %s into mod '%s'\n", addonPath, modName)
	fmt.Printf("  File: %s\n", modFilePath)
	return nil
}

// runModAddonRemove removes an addon file override from a mod (reverts to baseline).
func runModAddonRemove(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod addon remove <path> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	addonPath := filepath.ToSlash(remaining[0])

	modFilePath := filepath.Join(cfg.ModAddonsDir(modName), addonPath)
	if _, err := os.Stat(modFilePath); os.IsNotExist(err) {
		return fmt.Errorf("addon file not found in mod '%s': %s", modName, addonPath)
	}

	if err := os.Remove(modFilePath); err != nil {
		return fmt.Errorf("remove addon file: %w", err)
	}

	// Clean up empty parent directories
	cleanEmptyDirs(cfg.ModAddonsDir(modName))

	fmt.Printf("✓ Removed %s from mod '%s' (will use baseline version)\n", addonPath, modName)
	return nil
}

func runModAddonList(args []string) error {
	cfg := DefaultConfig()

	if _, err := os.Stat(cfg.BaselineAddonsDir); os.IsNotExist(err) {
		fmt.Println("No baseline addon files found. Run 'mithril mod init' first.")
		return nil
	}

	// Group files by addon directory
	addons := make(map[string][]string)

	err := filepath.Walk(cfg.BaselineAddonsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(cfg.BaselineAddonsDir, path)
		rel = filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(rel))
		if ext == ".lua" || ext == ".xml" || ext == ".toc" {
			// Group by the first two path components (e.g., "Interface/AddOns/Blizzard_AuctionUI")
			parts := strings.Split(rel, "/")
			group := rel
			if len(parts) >= 3 {
				group = strings.Join(parts[:3], "/")
			} else if len(parts) >= 2 {
				group = strings.Join(parts[:2], "/")
			}
			addons[group] = append(addons[group], rel)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk addons: %w", err)
	}

	// Sort and display
	var groups []string
	for g := range addons {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	totalFiles := 0
	fmt.Printf("%-55s %s\n", "Addon / Directory", "Files")
	fmt.Println(strings.Repeat("-", 65))
	for _, g := range groups {
		files := addons[g]
		fmt.Printf("%-55s %d\n", g, len(files))
		totalFiles += len(files)
	}
	fmt.Printf("\nTotal: %d addon directories, %d files\n", len(groups), totalFiles)

	return nil
}

func runModAddonSearch(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 {
		return fmt.Errorf("usage: mithril mod addon search <pattern> [--mod <name>]")
	}

	cfg := DefaultConfig()
	pattern := remaining[0]

	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	// If a mod is specified, search mod's addons first, then baseline for the rest
	// Otherwise just search baseline
	type searchResult struct {
		file    string
		matches []string
		source  string // "baseline" or mod name
	}

	var results []searchResult

	if modName != "" {
		modAddonsDir := cfg.ModAddonsDir(modName)
		modFiles := make(map[string]bool)

		// Search mod files
		if _, err := os.Stat(modAddonsDir); err == nil {
			filepath.Walk(modAddonsDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(modAddonsDir, path)
				rel = filepath.ToSlash(rel)
				modFiles[strings.ToLower(rel)] = true

				matches := searchFile(path, re)
				if len(matches) > 0 {
					results = append(results, searchResult{file: rel, matches: matches, source: modName})
				}
				return nil
			})
		}

		// Search baseline for non-overridden files
		filepath.Walk(cfg.BaselineAddonsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(cfg.BaselineAddonsDir, path)
			rel = filepath.ToSlash(rel)
			if modFiles[strings.ToLower(rel)] {
				return nil // already searched mod's version
			}
			matches := searchFile(path, re)
			if len(matches) > 0 {
				results = append(results, searchResult{file: rel, matches: matches, source: "baseline"})
			}
			return nil
		})
	} else {
		filepath.Walk(cfg.BaselineAddonsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(cfg.BaselineAddonsDir, path)
			rel = filepath.ToSlash(rel)
			matches := searchFile(path, re)
			if len(matches) > 0 {
				results = append(results, searchResult{file: rel, matches: matches, source: "baseline"})
			}
			return nil
		})
	}

	if len(results) == 0 {
		fmt.Printf("No matches found for pattern: %s\n", pattern)
		return nil
	}

	totalMatches := 0
	for _, r := range results {
		label := r.file
		if modName != "" {
			label = fmt.Sprintf("%s [%s]", r.file, r.source)
		}
		fmt.Printf("\n=== %s (%d matches) ===\n", label, len(r.matches))
		for _, m := range r.matches {
			fmt.Println(m)
		}
		totalMatches += len(r.matches)
	}
	fmt.Printf("\nTotal: %d matches across %d files\n", totalMatches, len(results))

	return nil
}

func runModAddonEdit(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod addon edit <path> --mod <mod_name>\n\nExample: mithril mod addon edit Interface/FrameXML/SpellBookFrame.lua --mod my-mod")
	}

	cfg := DefaultConfig()
	addonPath := remaining[0]
	addonPath = filepath.ToSlash(addonPath)

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Ensure the mod has a copy — copy from baseline if not
	modFilePath := filepath.Join(cfg.ModAddonsDir(modName), addonPath)
	if _, err := os.Stat(modFilePath); os.IsNotExist(err) {
		if err := copyBaselineAddonToMod(cfg, modName, addonPath); err != nil {
			return err
		}
		fmt.Printf("Copied %s from baseline to mod '%s'\n", addonPath, modName)
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
		fmt.Printf("Addon file is at: %s\n", modFilePath)
		fmt.Println("Set $EDITOR to your preferred editor and try again.")
		return nil
	}

	fmt.Printf("Opening %s in %s (mod: %s)...\n", addonPath, editor, modName)

	cmd := exec.Command(editor, modFilePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	fmt.Println("File saved.")
	fmt.Printf("Run 'mithril mod build --mod %s' to build the patch MPQs.\n", modName)

	return nil
}

// --- Addon helpers ---

func copyBaselineAddonToMod(cfg *Config, modName, addonPath string) error {
	baselinePath := filepath.Join(cfg.BaselineAddonsDir, addonPath)
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		return fmt.Errorf("addon file %q not found in baseline (run 'mithril mod init' first)", addonPath)
	}

	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return fmt.Errorf("read baseline addon: %w", err)
	}

	destPath := filepath.Join(cfg.ModAddonsDir(modName), addonPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create mod addon dir: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("write mod addon: %w", err)
	}

	return nil
}

// searchFile searches a text file for lines matching a regex.
func searchFile(path string, re *regexp.Regexp) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var matches []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			display := line
			if len(display) > 150 {
				display = display[:150] + "..."
			}
			matches = append(matches, fmt.Sprintf("  line %d: %s", lineNum, display))
			if len(matches) >= 10 {
				matches = append(matches, "  ... (showing first 10 matches per file)")
				break
			}
		}
	}

	return matches
}

// findModifiedAddons returns addon file paths that differ from baseline in a mod.
func findModifiedAddons(cfg *Config, modName string) []string {
	modAddonsDir := cfg.ModAddonsDir(modName)
	if _, err := os.Stat(modAddonsDir); os.IsNotExist(err) {
		return nil
	}

	var modified []string
	filepath.Walk(modAddonsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(modAddonsDir, path)
		rel = filepath.ToSlash(rel)

		baselinePath := filepath.Join(cfg.BaselineAddonsDir, rel)
		if !filesEqual(path, baselinePath) {
			modified = append(modified, rel)
		}
		return nil
	})

	sort.Strings(modified)
	return modified
}
