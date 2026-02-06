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
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Print(usage)
		return fmt.Errorf("unknown command: %s", args[0])
	}
}
