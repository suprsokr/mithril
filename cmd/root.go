package cmd

import (
	"fmt"
)

const usage = `Mithril - WoW 3.3.5a TrinityCore Dev Server CLI

Usage:
  mithril <command> [subcommand]

Commands:
  init             Initialize the Mithril dev environment (build Docker image,
                   clone TrinityCore, compile, extract maps, setup database)
  server start     Start the TrinityCore server containers
  server stop      Stop the TrinityCore server containers
  server restart   Restart the TrinityCore server containers
  server status    Show status of the TrinityCore server containers
  server attach    Attach to the worldserver console
  server logs      Stream container logs (Ctrl+C to stop)
  server account create <user> <pass> [gm_level]
                   Create a game account (gm_level: 0-3, default 3)
  client start     Launch the WoW 3.3.5a client (via Wine on Linux/macOS)

  mod init         Extract baseline DBCs from client MPQs
  mod create       Create a new named mod
  mod list         List all mods
  mod status       Show which DBCs have been modified
  mod build        Build patch-M.MPQ (one mod or all)
  mod dbc list     List all baseline DBC files
  mod dbc search   Search across DBC CSVs (regex)
  mod dbc inspect  Show schema and sample records for a DBC
  mod dbc edit     Open a DBC CSV in $EDITOR (per mod)
  mod dbc set      Programmatically edit a DBC field (per mod)
  mod addon list   List all baseline addon files
  mod addon search Search addon files (regex)
  mod addon edit   Edit an addon file in a mod
  mod patch list   List available binary patches
  mod patch apply  Apply a binary patch to Wow.exe
  mod patch status Show applied binary patches
  mod patch restore Restore Wow.exe from clean backup
  mod sql create   Create a SQL migration
  mod sql apply    Apply pending SQL migrations
  mod sql list     List SQL migrations
  mod core apply   Apply TrinityCore core patches
  mod core list    List core patches

Flags:
  -h, --help       Show this help message
`

// Execute parses CLI arguments and dispatches to the appropriate command.
func Execute(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "server":
		if len(args) < 2 {
			fmt.Print(usage)
			return fmt.Errorf("server command requires a subcommand: start, stop, restart, status, attach, logs")
		}
		return runServer(args[1], args[2:])
	case "client":
		if len(args) < 2 {
			fmt.Print(usage)
			return fmt.Errorf("client command requires a subcommand: start")
		}
		return runClient(args[1], args[2:])
	case "mod":
		return runMod(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Print(usage)
		return fmt.Errorf("unknown command: %s", args[0])
	}
}
