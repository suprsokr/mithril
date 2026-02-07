package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/suprsokr/mithril/internal/dbc"
)

// openDBCDB opens a connection to the dbc MySQL database.
func openDBCDB(cfg *Config) (*sql.DB, error) {
	dbCfg := dbc.DBConfig{
		User:     cfg.MySQLUser,
		Password: cfg.MySQLPassword,
		Host:     cfg.MySQLHost(),
		Port:     cfg.MySQLPort(),
		Name:     "dbc",
	}
	return dbc.OpenDB(dbCfg)
}

// runModDBCImport imports all baseline DBC files into the MySQL dbc database.
func runModDBCImport(args []string) error {
	cfg := DefaultConfig()

	// Check baseline exists
	if _, err := os.Stat(cfg.BaselineDbcDir); os.IsNotExist(err) {
		return fmt.Errorf("baseline DBC directory not found at %s — run 'mithril mod init' first", cfg.BaselineDbcDir)
	}

	// Ensure dbc database exists (needs root credentials)
	rootCfg := dbc.DBConfig{
		User:     "root",
		Password: cfg.MySQLRootPassword,
		Host:     cfg.MySQLHost(),
		Port:     cfg.MySQLPort(),
	}
	if err := dbc.EnsureDatabase(rootCfg, cfg.MySQLUser); err != nil {
		return fmt.Errorf("ensure dbc database: %w", err)
	}

	// Open connection to dbc database
	db, err := openDBCDB(cfg)
	if err != nil {
		return fmt.Errorf("connect to dbc database: %w", err)
	}
	defer db.Close()

	// Parse --force flag
	force := false
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
		}
	}

	fmt.Printf("Importing DBC files from %s into MySQL...\n", cfg.BaselineDbcDir)

	imported, skipped, err := dbc.ImportAllDBCs(db, cfg.BaselineDbcDir, force)
	if err != nil {
		return fmt.Errorf("import DBCs: %w", err)
	}

	fmt.Printf("\n✓ Imported %d DBC tables (%d skipped)\n", imported, skipped)
	fmt.Println("\nYou can now query DBC data with SQL:")
	fmt.Println("  mithril mod dbc query \"SELECT id, name_enus, flags FROM areatable WHERE map_id = 0 LIMIT 5\"")
	fmt.Println("\nCreate DBC SQL migrations with:")
	fmt.Println("  mithril mod sql create <name> --mod <mod_name> --db dbc")

	return nil
}

// runModDBCQuery runs an ad-hoc SQL query against the dbc database.
func runModDBCQuery(args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: mithril mod dbc query "<SQL>"

Examples:
  mithril mod dbc query "SELECT id, name_enus, flags FROM areatable WHERE map_id IN (0,1) LIMIT 10"
  mithril mod dbc query "SHOW TABLES"
  mithril mod dbc query "DESCRIBE areatable"
  mithril mod dbc query "SELECT COUNT(*) FROM areatable WHERE flags & 1024"`)
		return fmt.Errorf("SQL query required")
	}

	cfg := DefaultConfig()

	db, err := openDBCDB(cfg)
	if err != nil {
		return fmt.Errorf("connect to dbc database: %w", err)
	}
	defer db.Close()

	sqlQuery := args[0]

	rows, err := db.Query(sqlQuery)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}

	// Print header
	fmt.Println(strings.Join(cols, "\t"))

	// Print rows
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		var parts []string
		for _, v := range vals {
			switch val := v.(type) {
			case nil:
				parts = append(parts, "NULL")
			case []byte:
				parts = append(parts, string(val))
			default:
				parts = append(parts, fmt.Sprintf("%v", val))
			}
		}
		fmt.Println(strings.Join(parts, "\t"))
	}

	return rows.Err()
}

// runModDBCExport exports modified DBC tables from MySQL back to .dbc binary files.
func runModDBCExport(args []string) error {
	cfg := DefaultConfig()

	db, err := openDBCDB(cfg)
	if err != nil {
		return fmt.Errorf("connect to dbc database: %w", err)
	}
	defer db.Close()

	metaFiles, err := dbc.GetEmbeddedMetaFiles()
	if err != nil {
		return fmt.Errorf("get meta files: %w", err)
	}

	exportDir := filepath.Join(cfg.ModulesDir, "build", "dbc_export")

	fmt.Println("Exporting modified DBC tables from MySQL...")

	exported, err := dbc.ExportModifiedDBCs(db, metaFiles, cfg.BaselineDbcDir, exportDir)
	if err != nil {
		return fmt.Errorf("export DBCs: %w", err)
	}

	if len(exported) == 0 {
		fmt.Println("No modified DBC tables detected.")
	} else {
		fmt.Printf("\n✓ Exported %d DBC files to %s\n", len(exported), exportDir)
	}

	return nil
}
