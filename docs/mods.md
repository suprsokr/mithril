# Mithril Modding Framework

Mithril provides an integrated modding framework for WoW 3.3.5a (WotLK). Mods are organized under the `modules/` directory, with each mod isolated in its own folder. A shared **baseline** extracted from the client serves as the unmodified reference for all mods.

## Concepts

### Baseline

The baseline is a pristine copy of every DBC and addon file extracted from the client's MPQ archives. It reflects the exact state of the game data as shipped. The baseline is:

- Created once with `mithril mod init`
- Stored in `modules/baseline/`
- Never edited directly
- The reference point for detecting what each mod has changed

### Mods

A mod is a named directory under `modules/` that contains only the files it modifies. When you first edit a file in a mod, the baseline copy is automatically duplicated into the mod's directory. From that point, the mod's copy diverges from the baseline.

A mod can contain any combination of:

- **DBC edits** — Game data changes (spells, items, talents, etc.) via SQL migrations
- **Addon edits** — UI modifications (Lua, XML, TOC files) with preserved directory structure
- **Binary patches** — Byte-level patches to `Wow.exe` described as JSON files
- **SQL migrations** — Database changes with forward + rollback pairs (server databases and DBC tables)
- **Core patches** — TrinityCore C++ code changes as git `.patch` files

Mods are:

- **Independent** — each mod has its own directory and tracks its own changes
- **Composable** — multiple mods can be built together into a single patch MPQ
- **Minimal** — a mod only contains the files it actually changes, not every file

### Patch Chain

WoW 3.3.5a loads data from MPQ archives in a specific order (the "patch chain"). Archives loaded later override files from earlier archives. `mithril mod build` always builds all mods together and generates:

- **`patch-M.MPQ`** (DBCs) in `modules/build/` and deployed to `client/Data/`
- **`patch-enUS-M.MPQ`** (addons) in `modules/build/` and deployed to `client/Data/enUS/`

DBC files go in non-locale MPQs (`Data/patch-M.MPQ`), while addon files go in locale-specific MPQs (`Data/enUS/patch-enUS-M.MPQ`) because the WoW client loads addon files from locale archives with higher priority. All letter-based patches sort after `patch-3.MPQ`, ensuring mod changes take priority over the base game.

The patch letter (default "M") can be customized in `mithril-data/mithril.json`:

```json
{"patch_letter": "Z"}
```

## Build Order

When multiple mods are built together, `mithril mod build` processes them in a defined order. Mods processed later override files from earlier mods when they modify the same file (DBC or addon).

The build order is stored in `modules/baseline/manifest.json` under the `build_order` key:

```json
{
  "build_order": ["base-fixes", "my-spell-mod", "my-item-mod"]
}
```

**Automatic ordering:** When you create a mod (`mithril mod create`) or install one (`mithril mod registry install`), it is automatically appended to `build_order`. This means mods are built in the order they were added — later mods take priority.

**Manual override:** You can reorder entries in `modules/baseline/manifest.json` to change priority. Mods listed later override earlier ones for conflicting files.

## Directory Structure

```
mithril-data/
├── client/Data/                    # WoW 3.3.5a client
│   ├── common.MPQ                  # Base game data
│   ├── patch.MPQ, patch-2.MPQ, patch-3.MPQ
│   └── patch-M.MPQ                 # ← Combined build, deployed by mithril mod build
│
└── modules/
    ├── baseline/                   # Shared pristine reference (never edit)
    │   ├── dbc/                    # Raw .dbc binaries from MPQ chain
    │   ├── addons/                 # Baseline addon files (lua/xml/toc)
    │   └── manifest.json           # Extraction metadata + build_order
    │
    ├── my-spell-mod/               # A named mod
    │   ├── mod.json                # Mod metadata (name, description, created_at)
    │   ├── addons/                 # Only the addon files this mod changes
    │   ├── binary-patches/         # Binary patches for Wow.exe
    │   ├── sql/                    # SQL migrations (forward + rollback pairs)
    │   │   ├── world/              # Server database migrations
    │   │   │   ├── 001_add_custom_npc.sql
    │   │   │   └── 001_add_custom_npc.rollback.sql
    │   │   └── dbc/                # DBC SQL migrations
    │   │       ├── 001_enable_flying.sql
    │   │       └── 001_enable_flying.rollback.sql
    │   └── core-patches/           # TrinityCore C++ patches
    │       └── 001_custom_handler.patch
    │
    ├── my-item-mod/                # Another mod
    │
    └── build/                      # Build artifacts
        ├── patch-M.MPQ             # Combined DBC MPQ (all mods)
        └── patch-enUS-M.MPQ        # Combined addon MPQ (all mods)
```

## Commands

| Command | Description |
|---|---|
| `mithril mod init` | Extract baseline from client MPQs (DBCs, addons) |
| `mithril mod create <name>` | Create a new named mod |
| `mithril mod list` | List all mods and their status |
| `mithril mod status [--mod <name>]` | Show what a mod has changed |
| `mithril mod build` | Build combined patch MPQs from all mods |

Each mod type has its own set of commands documented in the workflow guides:

- **[DBC Workflow](dbc-workflow.md)** — `mithril mod dbc *` — Editing game data (spells, items, talents)
- **[Addon Workflow](addons-workflow.md)** — `mithril mod addon *` — Modifying the client UI (Lua/XML)
- **[Binary Patches Workflow](binary-patches-workflow.md)** — `mithril mod patch *` — Patching the client executable
- **[SQL Workflow](sql-workflow.md)** — `mithril mod sql *` — Server-side database migrations
- **[Core Patches Workflow](core-patches-workflow.md)** — `mithril mod core *` — TrinityCore C++ patches
- **[Sharing Mods](sharing-mods.md)** — `mithril mod registry *` / `mithril mod publish *` — Discover, install, and share mods

## Supported Modding Workflows

### DBC Editing

DBC files define game data: spells, items, talents, creatures, maps, and more. See [DBC Workflow](dbc-workflow.md) for the full guide.

### Addon / UI Modding

The WoW client's built-in UI is implemented as Lua/XML addons inside the MPQ archives. Mithril extracts all 465+ addon files (`.lua`, `.xml`, `.toc`) from the client's locale MPQs to the baseline, and lets you override them per-mod. Addon modifications go into locale-specific MPQs (`patch-enUS-M.MPQ`) to ensure they have higher priority than the base UI files. See [Addon Workflow](addons-workflow.md) for the full guide.

### Binary Patches

Some mods require changes to the WoW client executable itself (`Wow.exe`). Binary patches are JSON files that describe byte-level changes at specific addresses.

Binary patches are treated like any other mod content — they live in a mod's `binary-patches/` directory and can be distributed and shared. See [Binary Patches Workflow](binary-patches-workflow.md) for the full guide.

### SQL Migrations

SQL migrations modify the TrinityCore server databases and the DBC table data. Mithril provides a numbered migration system with forward and rollback pairs. Each `sql create` generates both files. Rollback files are never auto-applied — use `sql rollback` to undo changes.

```bash
mithril mod sql create add_custom_npc --mod my-mod          # server (world) migration
mithril mod sql create enable_flying --mod my-mod --db dbc  # DBC migration
mithril mod sql apply --mod my-mod                          # apply pending
mithril mod sql rollback --mod my-mod --reapply             # undo + redo (for iterating)
```

DBC SQL migrations are also automatically applied during `mod build`. See [SQL Workflow](sql-workflow.md) for the full guide.

### TrinityCore Core Patches

Core patches modify the TrinityCore C++ server code — adding new features, custom spell handlers, chat commands, or core mechanic changes. Patches are standard git `.patch` files applied to the TrinityCore source tree before compilation.

```bash
mithril mod core apply --mod my-mod
mithril init --rebuild
```

See [Core Patches Workflow](core-patches-workflow.md) for the full guide.

### Sharing & Community

Mods can be shared through the **Mithril Mod Registry** — a community GitHub repository where mod authors register their mods. Users can search, browse, and install mods directly from the CLI. Installing a mod clones its git repository so you have the full source and can build locally with `mithril mod build`. See [Sharing Mods](sharing-mods.md) for the full guide.
