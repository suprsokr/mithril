package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runModDBC(subcmd string, args []string) error {
	switch subcmd {
	case "create":
		return runModDBCCreate(args)
	case "import":
		return runModDBCImport(args)
	case "query":
		return runModDBCQuery(args)
	case "export":
		return runModDBCExport(args)
	case "remove":
		return runModDBCRemove(args)
	case "-h", "--help", "help":
		fmt.Print(modUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod dbc command: %s", subcmd)
	}
}

// runModDBCRemove removes a DBC SQL migration. Shorthand for `mithril mod sql remove --db dbc`.
func runModDBCRemove(args []string) error {
	return runModSQLRemove(args)
}

// runModDBCCreate creates a DBC SQL migration. Shorthand for `mithril mod sql create --db dbc`.
func runModDBCCreate(args []string) error {
	// Inject --db dbc into the args
	return runModSQLCreate(append(args, "--db", "dbc"))
}

func findRawDBCFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".dbc") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}
