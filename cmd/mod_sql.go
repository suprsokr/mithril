package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func runModSQL(subcmd string, args []string) error {
	switch subcmd {
	case "list":
		return runModSQLList(args)
	case "apply":
		return runModSQLApply(args)
	case "create":
		return runModSQLCreate(args)
	case "status":
		return runModSQLStatus(args)
	case "-h", "--help", "help":
		fmt.Print(sqlUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod sql command: %s", subcmd)
	}
}

const sqlUsage = `Mithril Mod SQL - Server-side database migrations

Usage:
  mithril mod sql <command> [args]

Commands:
  create <name> --mod <mod> [--db <database>]
                            Create a new SQL migration file
  list [--mod <mod>]        List SQL migrations and their status
  apply [--mod <mod>]       Apply pending SQL migrations
  status [--mod <mod>]      Show migration status

Databases:
  world       Game world data (creatures, items, quests) [default]
  characters  Character data
  auth        Account and authentication data

Examples:
  mithril mod sql create add_custom_npc --mod my-mod
  mithril mod sql create set_xp_rate --mod my-mod --db world
  mithril mod sql list --mod my-mod
  mithril mod sql apply --mod my-mod
  mithril mod sql apply
`

// SQLTracker records which migrations have been applied.
type SQLTracker struct {
	Applied []AppliedMigration `json:"applied"`
}

// AppliedMigration tracks a single migration that has been run.
type AppliedMigration struct {
	Mod       string `json:"mod"`
	File      string `json:"file"`
	Database  string `json:"database"`
	AppliedAt string `json:"applied_at"`
}

func (t *SQLTracker) IsApplied(mod, file string) bool {
	for _, a := range t.Applied {
		if a.Mod == mod && a.File == file {
			return true
		}
	}
	return false
}

func loadSQLTracker(cfg *Config) (*SQLTracker, error) {
	path := filepath.Join(cfg.ModulesDir, "sql_migrations_applied.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SQLTracker{}, nil
		}
		return nil, err
	}
	var t SQLTracker
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func saveSQLTracker(cfg *Config, t *SQLTracker) error {
	path := filepath.Join(cfg.ModulesDir, "sql_migrations_applied.json")
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// migrationInfo describes a SQL migration file.
type migrationInfo struct {
	mod      string
	filename string
	database string // parsed from directory or filename
	path     string
}

// findMigrations discovers SQL files in a mod's sql/ directory.
// Structure: sql/<database>/<NNN>_<name>.sql  or  sql/<NNN>_<name>.sql (defaults to "world")
func findMigrations(cfg *Config, modName string) []migrationInfo {
	sqlDir := filepath.Join(cfg.ModDir(modName), "sql")
	if _, err := os.Stat(sqlDir); os.IsNotExist(err) {
		return nil
	}

	var migrations []migrationInfo

	// Check for database subdirectories
	entries, err := os.ReadDir(sqlDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// sql/world/*.sql, sql/auth/*.sql, sql/characters/*.sql
			db := entry.Name()
			subDir := filepath.Join(sqlDir, db)
			files, _ := os.ReadDir(subDir)
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".sql") {
					migrations = append(migrations, migrationInfo{
						mod:      modName,
						filename: f.Name(),
						database: db,
						path:     filepath.Join(subDir, f.Name()),
					})
				}
			}
		} else if strings.HasSuffix(entry.Name(), ".sql") {
			// sql/*.sql — defaults to "world" database
			migrations = append(migrations, migrationInfo{
				mod:      modName,
				filename: entry.Name(),
				database: "world",
				path:     filepath.Join(sqlDir, entry.Name()),
			})
		}
	}

	// Sort by filename (which should be numbered: 001_..., 002_..., etc.)
	sort.Slice(migrations, func(i, j int) bool {
		if migrations[i].database != migrations[j].database {
			return migrations[i].database < migrations[j].database
		}
		return migrations[i].filename < migrations[j].filename
	})

	return migrations
}

func runModSQLCreate(args []string) error {
	modName, remaining := parseModFlag(args)
	if modName == "" || len(remaining) < 1 {
		return fmt.Errorf("usage: mithril mod sql create <name> --mod <mod_name> [--db <database>]")
	}

	cfg := DefaultConfig()
	name := remaining[0]

	// Parse --db flag
	database := "world"
	for i := 1; i < len(remaining); i++ {
		if remaining[i] == "--db" && i+1 < len(remaining) {
			database = remaining[i+1]
			break
		}
	}

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Find next migration number
	existing := findMigrations(cfg, modName)
	nextNum := 1
	for _, m := range existing {
		if m.database == database {
			// Extract number from filename
			parts := strings.SplitN(m.filename, "_", 2)
			if len(parts) >= 1 {
				var n int
				if _, err := fmt.Sscanf(parts[0], "%d", &n); err == nil && n >= nextNum {
					nextNum = n + 1
				}
			}
		}
	}

	// Create the SQL file
	sqlDir := filepath.Join(cfg.ModDir(modName), "sql", database)
	if err := os.MkdirAll(sqlDir, 0755); err != nil {
		return fmt.Errorf("create sql directory: %w", err)
	}

	// Sanitize name for filename
	safeName := strings.ReplaceAll(strings.ToLower(name), " ", "_")
	filename := fmt.Sprintf("%03d_%s.sql", nextNum, safeName)
	filePath := filepath.Join(sqlDir, filename)

	template := fmt.Sprintf(`-- Migration: %s
-- Database: %s
-- Mod: %s
--
-- Description: TODO
--

`, name, database, modName)

	if err := os.WriteFile(filePath, []byte(template), 0644); err != nil {
		return fmt.Errorf("create migration file: %w", err)
	}

	fmt.Printf("✓ Created migration: %s\n", filepath.Join("sql", database, filename))
	fmt.Printf("  Edit: %s\n", filePath)
	fmt.Printf("  Apply: mithril mod sql apply --mod %s\n", modName)

	return nil
}

func runModSQLList(args []string) error {
	modName, _ := parseModFlag(args)
	cfg := DefaultConfig()
	tracker, _ := loadSQLTracker(cfg)

	var mods []string
	if modName != "" {
		mods = []string{modName}
	} else {
		mods = getAllMods(cfg)
	}

	totalMigrations := 0
	for _, mod := range mods {
		migrations := findMigrations(cfg, mod)
		if len(migrations) == 0 {
			continue
		}

		fmt.Printf("Mod '%s':\n", mod)
		for _, m := range migrations {
			status := "pending"
			if tracker.IsApplied(m.mod, m.filename) {
				status = "✓ applied"
			}
			fmt.Printf("  [%-10s] %-12s %s\n", status, m.database, m.filename)
		}
		totalMigrations += len(migrations)
		fmt.Println()
	}

	if totalMigrations == 0 {
		fmt.Println("No SQL migrations found.")
		fmt.Println("Create one with: mithril mod sql create <name> --mod <mod_name>")
	}

	return nil
}

func runModSQLStatus(args []string) error {
	return runModSQLList(args)
}

func runModSQLApply(args []string) error {
	modName, _ := parseModFlag(args)
	cfg := DefaultConfig()
	tracker, err := loadSQLTracker(cfg)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}

	var mods []string
	if modName != "" {
		mods = []string{modName}
	} else {
		mods = getAllMods(cfg)
	}

	// Check that the server container is running
	containerID, err := composeContainerID(cfg)
	if err != nil || containerID == "" {
		return fmt.Errorf("server container not running — start it with 'mithril server start'")
	}

	applied := 0
	for _, mod := range mods {
		migrations := findMigrations(cfg, mod)
		if len(migrations) == 0 {
			continue
		}

		for _, m := range migrations {
			if tracker.IsApplied(m.mod, m.filename) {
				continue
			}

			fmt.Printf("Applying %s/%s → %s...\n", m.mod, m.filename, m.database)

			// Read the SQL file
			sqlContent, err := os.ReadFile(m.path)
			if err != nil {
				fmt.Printf("  ⚠ Failed to read %s: %v\n", m.filename, err)
				continue
			}

			// Execute via docker exec
			if err := execSQL(cfg, containerID, m.database, string(sqlContent)); err != nil {
				fmt.Printf("  ⚠ Failed to apply %s: %v\n", m.filename, err)
				return fmt.Errorf("migration failed — stopping to prevent out-of-order execution")
			}

			tracker.Applied = append(tracker.Applied, AppliedMigration{
				Mod:       m.mod,
				File:      m.filename,
				Database:  m.database,
				AppliedAt: timeNow(),
			})

			fmt.Printf("  ✓ %s\n", m.filename)
			applied++
		}
	}

	// Save tracker
	if err := saveSQLTracker(cfg, tracker); err != nil {
		return fmt.Errorf("save tracker: %w", err)
	}

	if applied == 0 {
		fmt.Println("No pending migrations to apply.")
	} else {
		fmt.Printf("\n✓ Applied %d migration(s)\n", applied)
		fmt.Println("You may need to restart the server for some changes to take effect:")
		fmt.Println("  mithril server restart")
	}

	return nil
}

// execSQL runs a SQL string against a database inside the Docker container.
func execSQL(cfg *Config, containerID, database, sql string) error {
	cmd := exec.Command("docker", "exec", "-i", containerID,
		"mysql", "-u", cfg.MySQLUser, "-p"+cfg.MySQLPassword, database)
	cmd.Stdin = strings.NewReader(sql)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}
