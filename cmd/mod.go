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
  build [--mod <name>]      Build patch-M.MPQ (one mod or all)

  dbc list                  List all baseline DBC files
  dbc search <pattern> [--mod <name>]
                            Search across DBC CSVs (regex)
  dbc inspect <dbc>         Show schema and sample records
  dbc edit <dbc> --mod <name>
                            Open a DBC CSV in $EDITOR for a mod
  dbc set <dbc> --mod <name> --where <key>=<val> --set <col>=<val>
                            Programmatically edit a DBC field

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
                            Create a new SQL migration
  sql list [--mod <name>]   List SQL migrations
  sql apply [--mod <name>]  Apply pending SQL migrations
  sql status [--mod <name>] Show migration status

  core list [--mod <name>]
                            List TrinityCore core patches
  core apply [--mod <name>]
                            Apply core patches to TrinityCore
  core status [--mod <name>]
                            Show core patch status

Examples:
  mithril mod init
  mithril mod create my-spell-mod
  mithril mod dbc set Spell --mod my-spell-mod --where id=133 --set spell_name_enUS="Mithril Bolt"
  mithril mod build --mod my-spell-mod
  mithril mod addon list
  mithril mod addon search "SpellBook"
  mithril mod addon edit Interface/FrameXML/SpellBookFrame.lua --mod my-mod
  mithril mod sql create add_custom_npc --mod my-mod
  mithril mod sql apply --mod my-mod
  mithril mod core apply --mod my-mod
  mithril mod build
  mithril mod list
`

// ModMeta is the metadata stored in each mod's mod.json.
type ModMeta struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	PatchSlot   string `json:"patch_slot"`
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
			return fmt.Errorf("mod dbc requires a subcommand: list, search, inspect, edit, set")
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
	if _, err := os.Stat(cfg.BaselineCsvDir); os.IsNotExist(err) {
		return fmt.Errorf("baseline not found — run 'mithril mod init' first")
	}

	modDir := cfg.ModDir(modName)
	if _, err := os.Stat(modDir); err == nil {
		return fmt.Errorf("mod already exists: %s", modName)
	}

	// Create mod directory and dbc subdirectory
	if err := os.MkdirAll(cfg.ModDbcDir(modName), 0755); err != nil {
		return fmt.Errorf("create mod dir: %w", err)
	}

	// Assign a patch slot (A, B, C, ... L, AA, AB, ...)
	slot, err := nextPatchSlot(cfg)
	if err != nil {
		return fmt.Errorf("assign patch slot: %w", err)
	}

	// Write mod.json
	meta := ModMeta{
		Name:      modName,
		PatchSlot: slot,
		CreatedAt: timeNow(),
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(modDir, "mod.json"), data, 0644); err != nil {
		return fmt.Errorf("write mod.json: %w", err)
	}

	fmt.Printf("✓ Created mod: %s\n", modName)
	fmt.Printf("  Patch slot: patch-%s.MPQ\n", slot)
	fmt.Printf("  Directory:  %s\n", modDir)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  mithril mod dbc edit Spell --mod %s\n", modName)
	fmt.Printf("  mithril mod dbc set Spell --mod %s --where id=133 --set spell_name_enUS=\"My Spell\"\n", modName)
	fmt.Printf("  mithril mod build --mod %s\n", modName)

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
	if _, err := os.Stat(cfg.BaselineCsvDir); os.IsNotExist(err) {
		fmt.Println("Baseline not extracted. Run 'mithril mod init' first.")
		return nil
	}

	// Count baseline CSVs
	baselineCSVs, _ := findCSVFiles(cfg.BaselineCsvDir)
	fmt.Printf("Baseline: %d DBC files with known schemas\n\n", len(baselineCSVs))

	// List mods
	mods := listMods(cfg, entries)
	if len(mods) == 0 {
		fmt.Println("No mods created yet. Run 'mithril mod create <name>' to start.")
		return nil
	}

	fmt.Printf("%-25s %-12s %s\n", "Mod", "Patch Slot", "Modified DBCs")
	fmt.Println(strings.Repeat("-", 55))
	for _, mod := range mods {
		modDbcDir := cfg.ModDbcDir(mod)
		csvs, _ := findCSVFiles(modDbcDir)
		slot := "?"
		if meta, err := loadModMeta(cfg, mod); err == nil && meta.PatchSlot != "" {
			slot = "patch-" + meta.PatchSlot
		}
		fmt.Printf("%-25s %-12s %d\n", mod, slot, len(csvs))
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

// getAllMods returns all mod names.
func getAllMods(cfg *Config) []string {
	entries, err := os.ReadDir(cfg.ModulesDir)
	if err != nil {
		return nil
	}
	return listMods(cfg, entries)
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

// patchSlotSequence generates the sequence: A, B, C, ... L, AA, AB, ... AL, BA, ...
// M is reserved for the combined patch.
var reservedSlots = map[string]bool{"M": true}

func nextPatchSlot(cfg *Config) (string, error) {
	// Collect all slots already in use
	used := make(map[string]bool)
	mods := getAllMods(cfg)
	for _, mod := range mods {
		meta, err := loadModMeta(cfg, mod)
		if err == nil && meta.PatchSlot != "" {
			used[meta.PatchSlot] = true
		}
	}

	// Generate slots in order: A-L, then AA-AL, BA-BL, etc.
	for _, slot := range generateSlotSequence() {
		if !used[slot] && !reservedSlots[slot] {
			return slot, nil
		}
	}

	return "", fmt.Errorf("no available patch slots (too many mods)")
}

func generateSlotSequence() []string {
	letters := "ABCDEFGHIJKL"
	var slots []string

	// Single letter: A through L
	for _, c := range letters {
		slots = append(slots, string(c))
	}

	// Double letter: AA through LL
	for _, c1 := range letters {
		for _, c2 := range letters {
			slots = append(slots, string(c1)+string(c2))
		}
	}

	return slots
}
