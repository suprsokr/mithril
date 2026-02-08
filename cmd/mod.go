package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const modUsage = `Mithril Mod - Integrated Modding Framework

Usage:
  mithril mod <command> [subcommand] [args]

Commands:
  init                      Extract baseline DBCs from client MPQs
  create <name>             Create a new mod
  list                      List all mods and their status
  status [--mod <name>]     Show which DBCs a mod has changed
  build                     Build combined patch MPQ from all mods

  dbc list                  List all DBC tables
  dbc search <pattern>      Search across DBC tables
  dbc inspect <dbc>         Show schema and sample records
  dbc import                Import baseline DBCs into MySQL
  dbc query "<SQL>"         Run ad-hoc SQL against the DBC database
  dbc export                Export modified DBC tables to .dbc files

  addon list                List all baseline addon files
  addon search <pattern> [--mod <name>]
                            Search addon files (regex)
  addon edit <path> --mod <name>
                            Edit an addon file (lua/xml/toc)

  patch list                List available binary patches
  patch apply <name|path>   Apply a binary patch to Wow.exe
  patch status              Show applied binary patches
  patch restore             Restore Wow.exe from clean backup

  sql create <name> --mod <name> [--db <database>]
                            Create a forward + rollback migration pair
  sql list [--mod <name>]   List SQL migrations
  sql apply [--mod <name>]  Apply pending SQL migrations
  sql rollback --mod <name> [<migration>] [--reapply]
                            Roll back a migration
  sql status [--mod <name>] Show migration status

  core list [--mod <name>]
                            List TrinityCore core patches
  core apply [--mod <name>]
                            Apply core patches to TrinityCore
  core status [--mod <name>]
                            Show core patch status

  registry list             List all mods in the community registry
  registry search <query>   Search mods by name, tags, or description
  registry info <name>      Show detailed info about a registry mod
  registry install <name>   Clone a mod's source repo and set it up locally

  publish register --mod <name> --repo <url>
                            Generate a registry JSON for your mod
  publish export --mod <name>
                            Export pre-built client.zip/server.zip (optional)

Examples:
  mithril mod init
  mithril mod create my-spell-mod
  mithril mod dbc query "SELECT id, spell_name_enus FROM spell WHERE id = 133"
  mithril mod sql create rename_spell --mod my-spell-mod --db dbc
  mithril mod build
  mithril mod addon list
  mithril mod addon search "SpellBook"
  mithril mod addon edit Interface/FrameXML/SpellBookFrame.lua --mod my-mod
  mithril mod sql create add_custom_npc --mod my-mod
  mithril mod sql apply --mod my-mod
  mithril mod core apply --mod my-mod
  mithril mod registry search "flying"
  mithril mod registry install fly-in-azeroth
  mithril mod publish register --mod my-mod --repo https://github.com/user/my-mod
  mithril mod build
  mithril mod list
`

// ModMeta is the metadata stored in each mod's mod.json.
// This file is meant to be committed to version control and shared.
// Local-only state (like patch slot assignments) is stored separately.
type ModMeta struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
}


func runMod(args []string) error {
	if len(args) == 0 {
		fmt.Print(modUsage)
		return nil
	}

	switch args[0] {
	case "init":
		return runModInit(args[1:])
	case "create":
		return runModCreate(args[1:])
	case "list":
		return runModList(args[1:])
	case "status":
		return runModStatus(args[1:])
	case "build":
		return runModBuild(args[1:])
	case "dbc":
		if len(args) < 2 {
			fmt.Print(modUsage)
			return fmt.Errorf("mod dbc requires a subcommand: list, search, inspect, import, query, export")
		}
		return runModDBC(args[1], args[2:])
	case "addon":
		if len(args) < 2 {
			fmt.Print(modUsage)
			return fmt.Errorf("mod addon requires a subcommand: list, search, edit")
		}
		return runModAddon(args[1], args[2:])
	case "patch":
		if len(args) < 2 {
			fmt.Print(modUsage)
			return fmt.Errorf("mod patch requires a subcommand: list, apply, status, restore")
		}
		return runModPatch(args[1], args[2:])
	case "sql":
		if len(args) < 2 {
			fmt.Print(modUsage)
			return fmt.Errorf("mod sql requires a subcommand: create, list, apply, status")
		}
		return runModSQL(args[1], args[2:])
	case "core":
		if len(args) < 2 {
			fmt.Print(modUsage)
			return fmt.Errorf("mod core requires a subcommand: list, apply, status")
		}
		return runModCore(args[1], args[2:])
	case "registry":
		if len(args) < 2 {
			fmt.Print(modUsage)
			return fmt.Errorf("mod registry requires a subcommand: list, search, info, install")
		}
		return runModRegistry(args[1], args[2:])
	case "publish":
		return runModPublish(args[1:])
	case "-h", "--help", "help":
		fmt.Print(modUsage)
		return nil
	default:
		fmt.Print(modUsage)
		return fmt.Errorf("unknown mod command: %s", args[0])
	}
}

func runModCreate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mithril mod create <name>")
	}

	cfg := DefaultConfig()
	modName := args[0]

	// Validate name
	if modName == "baseline" || modName == "build" {
		return fmt.Errorf("reserved name: %s", modName)
	}
	if strings.ContainsAny(modName, "/\\. ") {
		return fmt.Errorf("mod name cannot contain slashes, dots, or spaces: %s", modName)
	}

	// Check baseline exists
	if _, err := os.Stat(cfg.BaselineDbcDir); os.IsNotExist(err) {
		return fmt.Errorf("baseline not found — run 'mithril mod init' first")
	}

	modDir := cfg.ModDir(modName)
	if _, err := os.Stat(modDir); err == nil {
		return fmt.Errorf("mod already exists: %s", modName)
	}

	// Create mod directory
	if err := os.MkdirAll(modDir, 0755); err != nil {
		return fmt.Errorf("create mod dir: %w", err)
	}

	// Write mod.json (no patch slot — assigned at build time)
	meta := ModMeta{
		Name:      modName,
		CreatedAt: timeNow(),
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(modDir, "mod.json"), data, 0644); err != nil {
		return fmt.Errorf("write mod.json: %w", err)
	}

	// Add to build order in manifest
	if err := addModToBuildOrder(cfg, modName); err != nil {
		fmt.Printf("  ⚠ Failed to update build order: %v\n", err)
	}

	fmt.Printf("✓ Created mod: %s\n", modName)
	fmt.Printf("  Directory:  %s\n", modDir)

	return nil
}

func runModList(args []string) error {
	cfg := DefaultConfig()

	entries, err := os.ReadDir(cfg.ModulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No modules directory. Run 'mithril mod init' first.")
			return nil
		}
		return err
	}

	// Check baseline
	if _, err := os.Stat(cfg.BaselineDbcDir); os.IsNotExist(err) {
		fmt.Println("Baseline not extracted. Run 'mithril mod init' first.")
		return nil
	}

	// List mods
	mods := listMods(cfg, entries)
	if len(mods) == 0 {
		fmt.Println("No mods created yet. Run 'mithril mod create <name>' to start.")
		return nil
	}

	fmt.Printf("%-25s %s\n", "Mod", "SQL Migrations")
	fmt.Println(strings.Repeat("-", 40))
	for _, mod := range mods {
		migrations := findMigrations(cfg, mod)
		fmt.Printf("%-25s %d\n", mod, len(migrations))
	}

	return nil
}

// listMods returns names of all mods (directories under modules/ that have mod.json).
func listMods(cfg *Config, entries []os.DirEntry) []string {
	var mods []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "baseline" || name == "build" || strings.HasPrefix(name, ".") {
			continue
		}
		modJson := filepath.Join(cfg.ModDir(name), "mod.json")
		if _, err := os.Stat(modJson); err == nil {
			mods = append(mods, name)
		}
	}
	return mods
}

// getAllMods returns all mod names in build order.
// If the manifest has a build_order, mods are returned in that order first,
// followed by any mods on disk not in the list (alphabetically).
// This ensures explicit ordering is respected while remaining backward-compatible.
func getAllMods(cfg *Config) []string {
	entries, err := os.ReadDir(cfg.ModulesDir)
	if err != nil {
		return nil
	}
	diskMods := listMods(cfg, entries)

	manifest, err := loadManifest(cfg.BaselineDir)
	if err != nil || len(manifest.BuildOrder) == 0 {
		return diskMods
	}

	// Build a set of mods that actually exist on disk
	diskSet := make(map[string]bool, len(diskMods))
	for _, m := range diskMods {
		diskSet[m] = true
	}

	// Start with build_order entries that exist on disk
	seen := make(map[string]bool)
	var ordered []string
	for _, name := range manifest.BuildOrder {
		if diskSet[name] && !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}

	// Append any disk mods not in build_order (alphabetically, since diskMods is from ReadDir)
	for _, name := range diskMods {
		if !seen[name] {
			ordered = append(ordered, name)
		}
	}

	return ordered
}

// parseModFlag extracts --mod <name> from args, returning the mod name and remaining args.
// For commands that only support a single --mod, use this.
func parseModFlag(args []string) (string, []string) {
	modName := ""
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--mod" && i+1 < len(args) {
			modName = args[i+1]
			i++
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return modName, remaining
}

// parseModFlags extracts one or more --mod <name> flags from args.
func parseModFlags(args []string) ([]string, []string) {
	var mods []string
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--mod" && i+1 < len(args) {
			mods = append(mods, args[i+1])
			i++
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return mods, remaining
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// loadModMeta reads a mod's mod.json.
func loadModMeta(cfg *Config, modName string) (*ModMeta, error) {
	data, err := os.ReadFile(filepath.Join(cfg.ModDir(modName), "mod.json"))
	if err != nil {
		return nil, err
	}
	var meta ModMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

