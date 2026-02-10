# Mithril — Agent Orientation

Mithril is a CLI tool and integrated development environment for creating mods for WoW 3.3.5a (WotLK) running on TrinityCore. It manages a Docker-based TrinityCore server, extracts game data from the WoW client, and provides workflows for editing that data and packaging it back into the client and server.

This document orients AI agents working in this codebase. Read it first, then follow links for details.

## Key Concepts

- **Baseline** — Pristine game data extracted from the WoW client MPQs. Stored in `mithril-data/modules/baseline/`. Never edited directly.
- **Mod** — A named directory under `mithril-data/modules/<mod-name>/` containing only the files the mod changes. Mods are independent, composable, and minimal.
- **Patch chain** — WoW loads MPQs in order; later archives override earlier ones. Letter-based patches sort after `patch-3.MPQ`, so mod changes always win.
- **Build** — `mithril mod build` always builds all mods together into combined MPQ archives (`patch-M.MPQ`, `patch-enUS-M.MPQ`) for the client and flat DBC files for the server. The patch letter (default "M") is configurable via `mithril-data/mithril.json`.
- **Build order** — Mods are built in the order listed in `modules/manifest.json` under `build_order`. Later mods override earlier ones for conflicting files. Mods are automatically appended to `build_order` when created or installed. Users can reorder by editing the manifest. Mods on disk but not in the list are appended alphabetically.

## Lifecycle: From Zero to Running Mod

```
mithril init                          # 1. One-time setup (Docker image, compile TC, extract maps, setup DB)
mithril server start                  # 2. Start the TrinityCore server
mithril server account create admin admin  # 3. Create a GM account

mithril mod init                      # 4. Extract baseline DBCs/addons + import into MySQL
mithril mod create my-mod             # 5. Create a mod

# --- Make changes (any combination) ---
mithril mod dbc create my_dbc_change --mod my-mod            # DBC edit via SQL
mithril mod addon edit Interface/FrameXML/SpellBookFrame.lua --mod my-mod
mithril mod sql create add_npc --mod my-mod

# --- Build and deploy ---
mithril mod build                     # 6. Build MPQs, deploy to client + server
mithril mod sql apply --mod my-mod    # 7. Apply SQL migrations (server must be running)
mithril server restart                # 8. Restart server for DBC/SQL changes to take effect
```

Client-only changes (addon edits, cosmetic DBC changes like names/icons) don't need a server restart — just restart the WoW client or `/reload` in-game.

## Mod Content Types

A single mod can contain any combination of these. All live under `mithril-data/modules/<mod-name>/`.

| Type | Directory | Affects | Restart needed? | Workflow doc |
|---|---|---|---|---|
| DBC edits | `dbc/` | Client + server | Client restart; server restart if gameplay-affecting | [dbc-workflow.md](dbc-workflow.md) |
| Addon/UI edits | `addons/` | Client only | Client restart or `/reload` | [addons-workflow.md](addons-workflow.md) |
| SQL migrations | `sql/<db>/` | Server only | Some changes immediate, some need restart | [sql-workflow.md](sql-workflow.md) |
| Binary patches | `binary-patches/` | Client executable | One-time apply, survives builds | [binary-patches-workflow.md](binary-patches-workflow.md) |
| Core patches | `core-patches/` | Server C++ code | `mithril server rebuild` + restart | [core-patches-workflow.md](core-patches-workflow.md) |
| Scripts | `scripts/` | Server C++ code | `mithril server rebuild` + restart | [scripts-workflow.md](scripts-workflow.md) |

## Command Reference

### Server & Docker

```bash
mithril init                    # Build Docker image, compile TrinityCore, extract maps, setup DB
mithril server rebuild          # Recompile TrinityCore (incremental, for core patches / scripts)
mithril server start            # Start server containers (docker compose up -d)
mithril server stop             # Stop server containers (docker compose down)
mithril server restart          # Restart server containers (docker compose restart)
mithril server status           # Show container status (docker compose ps)
mithril server attach           # Attach to worldserver console (Ctrl+P, Ctrl+Q to detach)
mithril server logs             # Stream container logs (Ctrl+C to stop)
mithril server account create <user> <pass> [gm_level]  # Create game account (gm_level 0-3, default 3)
mithril client start            # Launch the WoW client (via Wine on Linux/macOS)
```

The server runs as a single Docker container (`mithril-server`) via Docker Compose. MySQL, authserver, and worldserver all run inside it. Ports: auth `3724`, world `8085`, MySQL `3306`.

### Mod Management

```bash
mithril mod init                          # Extract baseline from client MPQs
mithril mod create <name>                 # Create a new mod
mithril mod remove <name>                 # Remove a mod (directory, build order, trackers)
mithril mod list                          # List all mods
mithril mod status [--mod <name>]         # Show what changed vs baseline
mithril mod build                         # Build combined patch MPQs and deploy
```

### DBC Editing

```bash
mithril mod dbc create <name> --mod <mod>  # Create a DBC SQL migration
mithril mod dbc query "<SQL>"             # Ad-hoc SQL against the dbc database
mithril mod dbc export                    # Export modified tables to .dbc files
mithril mod dbc import                    # Re-import (only needed if DB was reset)
mithril mod dbc create <name> --mod <name>           # Create a DBC SQL migration
```

`mod init` automatically imports all baseline DBCs into MySQL tables (e.g., `areatable`, `spell`, `map`).
Mods create SQL migrations in `sql/dbc/` that run UPDATE/INSERT/DELETE statements.
`mod build` automatically applies pending DBC SQL migrations and exports only the touched tables.

### Addon/UI Editing

```bash
mithril mod addon create <path> --mod <name>       # Copy baseline addon file into mod for editing
mithril mod addon remove <path> --mod <name>       # Remove addon override (revert to baseline)
mithril mod addon list                    # List all baseline addon files (465+)
mithril mod addon search <pattern> [--mod <name>]  # Regex search addon files
mithril mod addon edit <path> --mod <name>         # Edit addon file (copies from baseline on first edit)
```

**Note:** Modifying `Interface/GlueXML/` or `Interface/FrameXML/` requires a binary patch to Wow.exe to disable the client's interface integrity check, or the client will crash.

### SQL Migrations

```bash
mithril mod sql create <name> --mod <name> [--db <database>]  # Create forward + rollback pair
mithril mod sql remove <migration> --mod <name>              # Remove migration (prompts rollback)
mithril mod sql list [--mod <name>]       # List migrations with applied/pending status
mithril mod sql apply [--mod <name>]      # Apply pending forward migrations
mithril mod sql rollback --mod <name> [<migration>] [--reapply]  # Roll back (and optionally re-apply)
```

Databases: `world` (default — creatures, items, quests), `characters`, `auth`, `dbc` (DBC table data). Server SQL runs via `docker exec`; DBC SQL connects directly to MySQL on port 3306. DBC migrations are also automatically applied and exported during `mod build`.

`sql create` generates a pair: `NNN_name.sql` (forward) and `NNN_name.rollback.sql` (rollback). Only forward files are auto-applied; rollback files are used by `sql rollback`.

### Binary Patches

```bash
mithril mod patch create <name> --mod <name>  # Scaffold a binary patch JSON file
mithril mod patch remove <name> --mod <name>  # Remove a patch (prompts to restore Wow.exe)
mithril mod patch list                    # List available patches from installed mods
mithril mod patch apply --mod <name>      # Apply all patches from a mod
mithril mod patch apply <path>            # Apply a specific patch JSON to Wow.exe
mithril mod patch status                  # Show applied patches
mithril mod patch restore                 # Restore Wow.exe from clean backup
```

Patches are distributed as mods with JSON files in their `binary-patches/` directories.

### Core Patches

```bash
mithril mod core create <name> --mod <name>  # Scaffold a core patch file
mithril mod core remove <name> --mod <name>  # Remove a core patch file
mithril mod core list [--mod <name>]      # List core patches
mithril mod core apply [--mod <name>]     # Apply .patch files to TrinityCore source
```

After applying core patches or scripts, rebuild with `mithril server rebuild` (incremental), then `mithril server restart`.

### Registry & Sharing

```bash
mithril mod registry list                 # List community mods
mithril mod registry search <query>       # Search by name, tags, description
mithril mod registry info <name>          # Show mod details
mithril mod registry install <name>       # Clone mod repo into modules/
mithril mod publish register --mod <name> --repo <url>  # Generate registry JSON
mithril mod publish export --mod <name>   # Export client.zip/server.zip artifacts
```

## Directory Layout

```
mithril-data/                           # Root data directory (created by mithril init)
├── client/Data/                        # WoW 3.3.5a client
│   ├── common.MPQ, patch.MPQ, ...      # Base game archives
│   └── patch-M.MPQ                     # ← Combined mod build output (DBC)
├── data/                               # Extracted maps, vmaps, mmaps, dbc for server
│   └── dbc/                            # Server-side DBC files (updated by build)
├── etc/                                # Server config (worldserver.conf, authserver.conf)
├── modules/
│   ├── baseline/                       # Pristine reference (NEVER edit)
│   │   ├── dbc/                        # Raw .dbc binaries
│   │   └── addons/                     # Addon files (lua/xml/toc)
│   ├── <mod-name>/                     # A mod
│   │   ├── mod.json                    # Metadata (name, description, created_at)
│   │   ├── addons/                     # Modified addon files only
│   │   ├── sql/world/                  # Server SQL migrations (forward + rollback pairs)
│   │   ├── sql/dbc/                    # DBC SQL migrations (forward + rollback pairs)
│   │   ├── binary-patches/             # Custom binary patches (JSON)
│   │   └── core-patches/              # TrinityCore C++ patches (.patch)
│   ├── build/                          # Build artifacts (combined MPQs)
│   ├── sql_migrations_applied.json     # Migration tracking
│   ├── core_patches_applied.json       # Core patch tracking
│   └── binary_patches_applied.json     # Binary patch tracking
├── docker-compose.yml
├── Dockerfile
├── mysql/                              # MySQL data (persisted)
├── log/                                # Server logs
├── tdb/                                # TDB full database download
└── TrinityCore/                        # TC source (inside Docker build context)
```

## Source Code Layout

The mithril CLI is a Go project at `mithril/`.

```
mithril/
├── main.go                             # Entry point → cmd.Execute()
├── cmd/
│   ├── root.go                         # Top-level command dispatch and usage text
│   ├── config.go                       # Config struct with all paths (DefaultConfig())
│   ├── init.go                         # mithril init (9-step setup)
│   ├── server.go                       # server start/stop/restart/status/attach/logs
│   ├── docker.go                       # Dockerfile, docker-compose.yml generation
│   ├── client.go                       # client start (Wine launcher)
│   ├── account.go                      # server account create (SRP6 auth)
│   ├── mod.go                          # mod subcommand dispatch, ModMeta struct
│   ├── mod_init.go                     # mod init (baseline extraction)
│   ├── mod_build.go                    # mod build (SQL→DBC, MPQ packaging, deploy)
│   ├── mod_dbc.go                      # mod dbc create/import/query/export
│   ├── mod_dbc_sql.go                  # mod dbc import/query/export (native MySQL driver)
│   ├── mod_addon.go                    # mod addon create/remove/list/search/edit
│   ├── mod_sql.go                      # mod sql create/remove/list/apply/rollback
│   ├── mod_core.go                     # mod core create/remove/list/apply
│   ├── mod_patch.go                    # mod patch create/remove/list/apply/status/restore
│   ├── mod_registry.go                 # mod registry list/search/info/install
│   ├── mod_publish.go                  # mod publish register/export
│   ├── helpers.go                      # Shared utilities (file ops, printing, flag parsing)
│   └── tcconfig.go                     # Server config file generation
├── internal/
│   ├── dbc/                            # DBC↔SQL codec (97 embedded .meta.json schemas)
│   │   ├── dbc_file.go                 # Binary DBC read/write
│   │   ├── db_connect.go               # MySQL connection helpers, TableName()
│   │   ├── db_import.go                # SQL import: DBC → MySQL (native driver, batched inserts, checksums)
│   │   ├── db_export.go                # SQL export: MySQL → DBC (native driver, CHECKSUM TABLE)
│   │   ├── meta_embed.go              # Embedded schema loader
│   │   └── meta/                       # 97 schema files (field names, types, array sizes)
│   └── patcher/                        # Binary patch engine
│       ├── patcher.go                  # Apply/restore byte patches to Wow.exe
│       └── builtin.go                  # Built-in patch definitions
└── docs/                               # Workflow documentation (you are here)
```

## Common Agent Tasks

### Creating a mod end-to-end

1. `mithril mod create <name>` — creates the mod directory and `mod.json`
2. Make changes using `addon create`, `sql create`, `dbc create`, `patch create`, `core create`, etc.
3. `mithril mod build` — builds all mods and deploys to client + server
4. `mithril mod sql apply --mod <name>` — applies SQL migrations (if any)
5. `mithril server restart` — picks up server-side changes

### Editing DBC data programmatically

Use DBC SQL migrations. `mod init` imports all DBCs into MySQL automatically:

```bash
mithril mod dbc query "DESCRIBE areatable"                      # See schema
mithril mod dbc query "SELECT id, name_enus, flags FROM areatable WHERE map_id = 0 LIMIT 5"  # Explore data
mithril mod dbc create enable_flying --mod my-mod               # Create forward + rollback pair
# Edit both: sql/dbc/001_enable_flying.sql and sql/dbc/001_enable_flying.rollback.sql
mithril mod build                        # Applies migration + exports + packages

# Iterating? Edit the .sql file, then:
mithril mod sql rollback --mod my-mod --reapply
mithril mod build
```

### Writing SQL migrations

```bash
mithril mod sql create <descriptive_name> --mod <name> [--db world|auth|characters|dbc]
```

This creates a pair: `NNN_<name>.sql` (forward) and `NNN_<name>.rollback.sql` (rollback). Edit both files. Use high entry IDs (100000+) for custom content to avoid conflicts. The server must be running for `sql apply`. To iterate, use `mithril mod sql rollback --mod <name> --reapply`.

### Searching for game data

```bash
mithril mod dbc query "SELECT id, spell_name_enus FROM spell WHERE spell_name_enus LIKE '%Fireball%'"
mithril mod addon search "SpellButton"   # Search all addon Lua/XML/TOC files
```

### Understanding the build output

`mithril mod build` always builds all mods together. It does the following:
1. Applies pending DBC SQL migrations
2. Exports modified DBC tables → binary `.dbc` files (comparing checksums against baseline for change detection)
3. Creates combined MPQs (`patch-M.MPQ` for DBCs, `patch-enUS-M.MPQ` for addons) in `modules/build/`
4. Deploys DBC MPQ → `client/Data/`, addon MPQ → `client/Data/enUS/`
5. Copies modified `.dbc` files → server's `data/dbc/` directory

## Further Reading

- [mods.md](mods.md) — Modding framework overview, concepts, directory structure
- [dbc-workflow.md](dbc-workflow.md) — DBC editing (spells, items, talents, maps)
- [addons-workflow.md](addons-workflow.md) — Client UI modding (Lua/XML)
- [sql-workflow.md](sql-workflow.md) — Server database migrations
- [binary-patches-workflow.md](binary-patches-workflow.md) — Client executable patching
- [core-patches-workflow.md](core-patches-workflow.md) — TrinityCore C++ patches
- [sharing-mods.md](sharing-mods.md) — Registry, publishing, distribution
- [server_admin.md](server_admin.md) — Server configuration, Docker, accounts, ports
