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
	case "rollback":
		return runModSQLRollback(args)
	case "status":
		return runModSQLStatus(args)
	case "remove":
		return runModSQLRemove(args)
	case "-h", "--help", "help":
		fmt.Print(sqlUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod sql command: %s", subcmd)
	}
}

// runModSQLRemove removes a SQL migration pair (forward + rollback) from a mod.
// If the migration was applied, prompts to execute the rollback script first.
func runModSQLRemove(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod sql remove <migration> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	target := remaining[0]

	// Find the migration
	migrations := findMigrations(cfg, modName)
	var found *migrationInfo
	for i, m := range migrations {
		name := strings.TrimSuffix(m.filename, ".sql")
		if m.filename == target || name == target || m.filename == target+".sql" {
			found = &migrations[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("migration '%s' not found in mod '%s'", target, modName)
	}

	// Check if applied — offer to run rollback
	tracker, _ := loadSQLTracker(cfg)
	if tracker.IsApplied(found.mod, found.filename) {
		rollbackPath := strings.TrimSuffix(found.path, ".sql") + ".rollback.sql"
		hasRollback := fileExists(rollbackPath)

		if hasRollback {
			fmt.Printf("Migration '%s' is currently applied to '%s'.\n", found.filename, found.database)
			if promptYesNo("Run the rollback script to undo changes?") {
				fmt.Printf("Rolling back %s/%s → %s...\n", found.mod, found.filename, found.database)
				sqlContent, err := os.ReadFile(rollbackPath)
				if err != nil {
					return fmt.Errorf("read rollback file: %w", err)
				}
				if err := runSQL(cfg, found.database, string(sqlContent)); err != nil {
					return fmt.Errorf("execute rollback: %w", err)
				}
				fmt.Printf("  ✓ Rolled back %s\n", found.filename)
			} else {
				fmt.Println("  Skipping rollback — changes will remain in the database.")
			}
		} else {
			fmt.Printf("  ⚠ Migration '%s' is applied but no rollback script found.\n", found.filename)
			fmt.Println("  Changes will remain in the database.")
		}

		// Remove from tracker regardless
		tracker.Unapply(found.mod, found.filename)
		saveSQLTracker(cfg, tracker)
	}

	// Remove forward file
	if err := os.Remove(found.path); err != nil {
		return fmt.Errorf("remove migration file: %w", err)
	}
	fmt.Printf("  Removed: %s\n", found.path)

	// Remove rollback file if it exists
	rollbackPath := strings.TrimSuffix(found.path, ".sql") + ".rollback.sql"
	if _, err := os.Stat(rollbackPath); err == nil {
		os.Remove(rollbackPath)
		fmt.Printf("  Removed: %s\n", rollbackPath)
	}

	// Clean up empty directories
	cleanEmptyDirs(filepath.Join(cfg.ModDir(modName), "sql"))

	fmt.Printf("✓ Removed migration: %s\n", found.filename)
	return nil
}

const sqlUsage = `Mithril Mod SQL - Database migrations

Usage:
  mithril mod sql <command> [args]

Commands:
  create <name> --mod <mod> [--db <database>]
                            Create a forward + rollback migration pair
  remove <migration> --mod <mod>
                            Remove a migration (forward + rollback files)
  list [--mod <mod>]        List SQL migrations and their status
  apply [--mod <mod>]       Apply pending SQL migrations
  rollback --mod <mod> [<migration>] [--reapply]
                            Roll back a migration using its .rollback.sql
  status [--mod <mod>]      Show migration status

Databases:
  world       Game world data (creatures, items, quests) [default]
  characters  Character data
  auth        Account and authentication data
  dbc         DBC table data (imported from client, used by mod build)

Files created by 'sql create':
  NNN_name.sql              Forward migration (applied automatically)
  NNN_name.rollback.sql     Rollback migration (applied manually if needed)

Rollback:
  Roll back the most recent migration for a mod:
    mithril mod sql rollback --mod my-mod

  Roll back a specific migration:
    mithril mod sql rollback --mod my-mod 001_enable_flying

  Roll back and immediately re-apply the forward migration:
    mithril mod sql rollback --mod my-mod --reapply
    mithril mod sql rollback --mod my-mod 001_enable_flying --reapply

Examples:
  mithril mod sql create add_custom_npc --mod my-mod
  mithril mod sql create enable_flying --mod my-mod --db dbc
  mithril mod sql list --mod my-mod
  mithril mod sql apply --mod my-mod
  mithril mod sql rollback --mod my-mod --reapply
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

// Unapply removes a migration from the tracker.
func (t *SQLTracker) Unapply(mod, file string) {
	var kept []AppliedMigration
	for _, a := range t.Applied {
		if !(a.Mod == mod && a.File == file) {
			kept = append(kept, a)
		}
	}
	t.Applied = kept
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
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".sql") && !strings.HasSuffix(f.Name(), ".rollback.sql") {
					migrations = append(migrations, migrationInfo{
						mod:      modName,
						filename: f.Name(),
						database: db,
						path:     filepath.Join(subDir, f.Name()),
					})
				}
			}
		} else if strings.HasSuffix(entry.Name(), ".sql") && !strings.HasSuffix(entry.Name(), ".rollback.sql") {
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
	forwardFilename := fmt.Sprintf("%03d_%s.sql", nextNum, safeName)
	rollbackFilename := fmt.Sprintf("%03d_%s.rollback.sql", nextNum, safeName)
	forwardPath := filepath.Join(sqlDir, forwardFilename)
	rollbackPath := filepath.Join(sqlDir, rollbackFilename)

	forwardTemplate := fmt.Sprintf(`-- Migration: %s
-- Database: %s
-- Mod: %s
--
-- Description: TODO
--

`, name, database, modName)

	rollbackTemplate := fmt.Sprintf(`-- Rollback: %s
-- Database: %s
-- Mod: %s
--
-- Undoes the changes made by %s
--

`, name, database, modName, forwardFilename)

	if err := os.WriteFile(forwardPath, []byte(forwardTemplate), 0644); err != nil {
		return fmt.Errorf("create migration file: %w", err)
	}
	if err := os.WriteFile(rollbackPath, []byte(rollbackTemplate), 0644); err != nil {
		return fmt.Errorf("create rollback file: %w", err)
	}

	fmt.Printf("✓ Created migration:\n")
	fmt.Printf("  Forward:  %s\n", forwardPath)
	fmt.Printf("  Rollback: %s\n", rollbackPath)
	fmt.Printf("  Apply:    mithril mod sql apply --mod %s\n", modName)

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

func runModSQLRollback(args []string) error {
	modName, remaining := parseModFlag(args)
	if modName == "" {
		return fmt.Errorf("usage: mithril mod sql rollback --mod <mod_name> [<migration>] [--reapply]")
	}

	cfg := DefaultConfig()

	// Parse --reapply flag and optional migration name
	reapply := false
	var targetMigration string
	for _, a := range remaining {
		if a == "--reapply" {
			reapply = true
		} else if !strings.HasPrefix(a, "--") {
			targetMigration = a
		}
	}

	tracker, err := loadSQLTracker(cfg)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}

	// Find applied migrations for this mod (in order)
	migrations := findMigrations(cfg, modName)
	var appliedMigrations []migrationInfo
	for _, m := range migrations {
		if tracker.IsApplied(m.mod, m.filename) {
			appliedMigrations = append(appliedMigrations, m)
		}
	}

	if len(appliedMigrations) == 0 {
		fmt.Printf("No applied migrations to roll back for mod '%s'.\n", modName)
		return nil
	}

	// Determine which migration to roll back
	var target migrationInfo
	if targetMigration != "" {
		// Find by name (with or without .sql extension, with or without number prefix)
		found := false
		for _, m := range appliedMigrations {
			name := strings.TrimSuffix(m.filename, ".sql")
			if m.filename == targetMigration || name == targetMigration || m.filename == targetMigration+".sql" {
				target = m
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("migration '%s' not found or not applied for mod '%s'", targetMigration, modName)
		}
	} else {
		// Default: most recent applied migration
		target = appliedMigrations[len(appliedMigrations)-1]
	}

	// Find the rollback file
	rollbackPath := strings.TrimSuffix(target.path, ".sql") + ".rollback.sql"
	if _, err := os.Stat(rollbackPath); os.IsNotExist(err) {
		return fmt.Errorf("rollback file not found: %s", rollbackPath)
	}

	// Run rollback
	fmt.Printf("Rolling back %s/%s → %s...\n", target.mod, target.filename, target.database)
	sqlContent, err := os.ReadFile(rollbackPath)
	if err != nil {
		return fmt.Errorf("read rollback file: %w", err)
	}
	if err := runSQL(cfg, target.database, string(sqlContent)); err != nil {
		return fmt.Errorf("execute rollback: %w", err)
	}

	// Remove from tracker
	tracker.Unapply(target.mod, target.filename)
	if err := saveSQLTracker(cfg, tracker); err != nil {
		return fmt.Errorf("save tracker: %w", err)
	}

	fmt.Printf("  ✓ Rolled back %s\n", target.filename)

	// Re-apply if requested
	if reapply {
		fmt.Printf("\nRe-applying %s/%s → %s...\n", target.mod, target.filename, target.database)
		sqlContent, err := os.ReadFile(target.path)
		if err != nil {
			return fmt.Errorf("read migration file: %w", err)
		}

		if err := runSQL(cfg, target.database, string(sqlContent)); err != nil {
			return fmt.Errorf("re-apply migration: %w", err)
		}

		tracker.Applied = append(tracker.Applied, AppliedMigration{
			Mod:       target.mod,
			File:      target.filename,
			Database:  target.database,
			AppliedAt: timeNow(),
		})
		if err := saveSQLTracker(cfg, tracker); err != nil {
			return fmt.Errorf("save tracker: %w", err)
		}

		fmt.Printf("  ✓ Re-applied %s\n", target.filename)
	}

	if target.database == "dbc" && reapply {
		fmt.Println("\nRun 'mithril mod build' to export the updated DBC and rebuild the patch.")
	} else if target.database == "dbc" {
		fmt.Println("\nRun 'mithril mod build' to export the updated DBC.")
	} else {
		fmt.Println("\nYou may need to restart the server:")
		fmt.Println("  mithril server restart")
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

	// Check which database types we need
	hasServerMigrations := false
	hasDBCMigrations := false
	for _, mod := range mods {
		for _, m := range findMigrations(cfg, mod) {
			if tracker.IsApplied(m.mod, m.filename) {
				continue
			}
			if m.database == "dbc" {
				hasDBCMigrations = true
			} else {
				hasServerMigrations = true
			}
		}
	}

	// Server container needed for world/auth/characters
	var containerID string
	if hasServerMigrations {
		containerID, err = composeContainerID(cfg)
		if err != nil || containerID == "" {
			return fmt.Errorf("server container not running — start it with 'mithril server start'")
		}
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

			sqlContent, err := os.ReadFile(m.path)
			if err != nil {
				fmt.Printf("  ⚠ Failed to read %s: %v\n", m.filename, err)
				continue
			}

			if err := runSQL(cfg, m.database, string(sqlContent)); err != nil {
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
		if hasServerMigrations {
			fmt.Println("You may need to restart the server for some changes to take effect:")
			fmt.Println("  mithril server restart")
		}
		if hasDBCMigrations {
			fmt.Println("Run 'mithril mod build' to export updated DBCs.")
		}
	}

	return nil
}

// runSQL executes a SQL string against the specified database.
// DBC database uses the native MySQL driver; server databases use docker exec.
func runSQL(cfg *Config, database, sqlStr string) error {
	if database == "dbc" {
		db, err := openDBCDB(cfg)
		if err != nil {
			return fmt.Errorf("connect to dbc database: %w", err)
		}
		defer db.Close()
		_, err = db.Exec(sqlStr)
		return err
	}

	// Server databases: use docker exec
	containerID, err := composeContainerID(cfg)
	if err != nil || containerID == "" {
		return fmt.Errorf("server container not running")
	}
	return execSQL(cfg, containerID, database, sqlStr)
}

// execSQL runs a SQL string against a database inside the Docker container.
func execSQL(cfg *Config, containerID, database, sqlStr string) error {
	cmd := exec.Command("docker", "exec", "-i", containerID,
		"mysql", "-u", cfg.MySQLUser, "-p"+cfg.MySQLPassword, database)
	cmd.Stdin = strings.NewReader(sqlStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}
