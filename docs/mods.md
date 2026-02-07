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

- **DBC edits** — Game data changes (spells, items, talents, etc.) stored as CSV files
- **Addon edits** — UI modifications (Lua, XML, TOC files) with preserved directory structure
- **Binary patches** — Byte-level patches to `Wow.exe` described as JSON files

Mods are:

- **Independent** — each mod has its own directory and tracks its own changes
- **Composable** — multiple mods can be built together into a single patch MPQ
- **Minimal** — a mod only contains the files it actually changes, not every file

### Patch Chain

WoW 3.3.5a loads data from MPQ archives in a specific order (the "patch chain"). Archives loaded later override files from earlier archives. Mithril assigns each mod a **patch slot** (A, B, C, ... L, then AA, AB, ...) and generates:

- **Per-mod DBC MPQs** (`patch-A.MPQ`, `patch-B.MPQ`, ...) in `modules/build/`
- **Per-mod addon MPQs** (`patch-enUS-A.MPQ`, ...) in `modules/build/` — locale-specific
- **Combined** `patch-M.MPQ` (DBCs) and `patch-enUS-M.MPQ` (addons), deployed to the client

DBC files go in non-locale MPQs (`Data/patch-M.MPQ`), while addon files go in locale-specific MPQs (`Data/enUS/patch-enUS-M.MPQ`) because the WoW client loads addon files from locale archives with higher priority. All letter-based patches sort after `patch-3.MPQ`, ensuring mod changes take priority over the base game.

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
    │   ├── csv/                    # Baseline CSVs (pristine exports)
    │   ├── addons/                 # Baseline addon files (lua/xml/toc)
    │   └── manifest.json           # Extraction metadata
    │
    ├── my-spell-mod/               # A named mod
    │   ├── mod.json                # Mod metadata
    │   ├── dbc/                    # Only the CSVs this mod changes
    │   │   └── Spell.dbc.csv
    │   ├── addons/                 # Only the addon files this mod changes
    │   │   └── Interface/GlueXML/GlueStrings.lua
    │   └── binary-patches/         # Binary patches for Wow.exe
    │       └── allow-custom-gluexml.json
    │
    ├── my-item-mod/                # Another mod
    │   ├── mod.json
    │   └── dbc/
    │       └── Item.dbc.csv
    │
    └── build/                      # Build artifacts
        ├── patch-A.MPQ             # Per-mod DBC MPQ (my-spell-mod)
        ├── patch-enUS-A.MPQ        # Per-mod addon MPQ (my-spell-mod)
        ├── patch-B.MPQ             # Per-mod DBC MPQ (my-item-mod)
        ├── patch-M.MPQ             # Combined DBC MPQ
        └── patch-enUS-M.MPQ        # Combined addon MPQ
```

## Commands

| Command | Description |
|---|---|
| `mithril mod init` | Extract baseline from client MPQs (DBCs, addons) |
| `mithril mod create <name>` | Create a new named mod |
| `mithril mod list` | List all mods and their status |
| `mithril mod status [--mod <name>]` | Show what a mod has changed |
| `mithril mod build [--mod <name>]` | Build patch MPQs from one or all mods |

Each mod type has its own set of commands documented in the workflow guides:

- **[DBC Workflow](dbc-workflow.md)** — `mithril mod dbc *` commands for editing game data (spells, items, talents)
- **[Addon Workflow](addons-workflow.md)** — `mithril mod addon *` commands for modifying the client UI (Lua/XML)
- **[Binary Patches Workflow](binary-patches-workflow.md)** — `mithril mod patch *` commands for patching the client executable

## Supported Modding Workflows

### DBC Editing

DBC files define game data: spells, items, talents, creatures, maps, and more. See [DBC Workflow](dbc-workflow.md) for the full guide.

### Addon / UI Modding

The WoW client's built-in UI is implemented as Lua/XML addons inside the MPQ archives. Mithril extracts all 465+ addon files (`.lua`, `.xml`, `.toc`) from the client's locale MPQs to the baseline, and lets you override them per-mod. Addon modifications go into locale-specific MPQs (`patch-enUS-M.MPQ`) to ensure they have higher priority than the base UI files. See [Addon Workflow](addons-workflow.md) for the full guide.

### Binary Patches

Some mods require changes to the WoW client executable itself (`Wow.exe`). For example, modifying GlueXML or FrameXML files requires disabling the client's interface integrity check. Binary patches are JSON files that describe byte-level changes at specific addresses.

Binary patches are treated like any other mod content — they live in a mod's `binary-patches/` directory and can be distributed and shared. Mithril also includes built-in patches for common needs. See [Binary Patches Workflow](binary-patches-workflow.md) for the full guide.

### Future Workflows

The modding framework is designed to support additional workflows as they're implemented:

- **SQL patches** — Server-side database changes (creature stats, loot tables, quest scripts)
- **Asset replacement** — Textures, models, and other art assets
- **Map editing** — ADT/WDT terrain modifications

Each workflow will follow the same pattern: mods are isolated directories, changes are tracked against a baseline, and output is packaged for the client or server as appropriate.
