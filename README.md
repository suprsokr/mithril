# Mithril

Integrated development environment for TrinityCore and WoW 3.3.5a modding.

### Managed Dev Server

One command to set up a fully compiled development-focused TrinityCore server with database, map extraction, and Docker orchestration.

### Every Kind of Mod

A single mod can span every layer of the game:

- **DBC edits** — modify client data tables via SQL
- **Addon overrides** — customize the client UI (Lua/XML)
- **Server scripts** — add custom NPCs, spells, commands, and event hooks (C++)
- **Core patches** — modify TrinityCore source code
- **SQL migrations** — world, auth, and characters database changes
- **Binary patches & DLLs** — patch the client executable

Mix and match mods and mod types easily.

### Share and Rollback

Mods are self-contained directories. Install community mods from [the registry](https://github.com/suprsokr/mithril-registry), or share your own. Every change is tracked — `mithril mod remove` rolls back SQL migrations, reverses core patches, removes scripts from the server, and restores the client binary. Clean undo, every time.

## Prerequisites

- **Go 1.22+** — [go.dev/dl](https://go.dev/dl/)
- **Docker** — [docs.docker.com/get-docker](https://docs.docker.com/get-docker/)
- **WoW 3.3.5a client** (build 12340) — you must supply your own copy
- **MySQL client libraries** — needed for DBC database operations

### Platform Notes

| Platform | Notes |
|----------|-------|
| Linux    | Fully supported. Docker must be running. |
| macOS    | Fully supported. Docker Desktop or Colima required. |
| Windows  | Use WSL2 with Docker Desktop. |

## Install

```bash
git clone --recursive https://github.com/suprsokr/mithril.git
cd mithril
go install .
```

## Uninstall

Remove the workspace and all Docker resources:

```bash
mithril clean --all
```

Then delete the binary:

```bash
rm $(which mithril)
```
