package dbc

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// TableName returns the SQL table name for a meta file.
func TableName(meta *MetaFile) string {
	if meta.TableName != "" {
		return strings.ToLower(meta.TableName)
	}
	return strings.ToLower(strings.TrimSuffix(meta.File, ".dbc"))
}

// DBConfig holds connection parameters for a MySQL database.
type DBConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	Name     string
}

// OpenDB opens a MySQL connection from a DBConfig.
func OpenDB(c DBConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&allowNativePasswords=true&multiStatements=true",
		c.User, c.Password, c.Host, c.Port, c.Name)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}

// EnsureDatabase creates the dbc database if it doesn't exist, using root credentials.
func EnsureDatabase(rootCfg DBConfig, dbcUser string) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true&allowNativePasswords=true",
		rootCfg.User, rootCfg.Password, rootCfg.Host, rootCfg.Port)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open root connection: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping root connection: %w", err)
	}

	stmts := []string{
		"CREATE DATABASE IF NOT EXISTS dbc DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		fmt.Sprintf("GRANT ALL PRIVILEGES ON dbc.* TO '%s'@'%%'", dbcUser),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON dbc.* TO '%s'@'localhost'", dbcUser),
		"FLUSH PRIVILEGES",
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			// Non-fatal: user might not exist on all hosts
			continue
		}
	}

	return nil
}
