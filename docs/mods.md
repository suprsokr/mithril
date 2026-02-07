# Mithril Modding Framework

Mithril provides an integrated modding framework for WoW 3.3.5a (WotLK). Mods are organized under the `modules/` directory, with each mod isolated in its own folder. A shared **baseline** extracted from the client serves as the unmodified reference for all mods.

## Concepts

### Baseline

The baseline is a pristine copy of every DBC file extracted from the client's MPQ archives. It reflects the exact state of the game data as shipped. The baseline is:

- Created once with `mithril mod init`
- Stored in `modules/baseline/`
- Never edited directly
- The reference point for detecting what each mod has changed

### Mods

A mod is a named directory under `modules/` that contains only the files it modifies. When you first edit a DBC in a mod, the baseline copy is automatically duplicated into the mod's directory. From that point, the mod's copy diverges from the baseline.

Mods are:

- **Independent** — each mod has its own directory and tracks its own changes
- **Composable** — multiple mods can be built together into a single patch MPQ
- **Minimal** — a mod only contains the DBC files it actually changes, not every DBC

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
    │   └── manifest.json           # Extraction metadata
    │
    ├── my-spell-mod/               # A named mod
    │   ├── mod.json                # Mod metadata
    │   └── dbc/                    # Only the CSVs this mod changes
    │       └── Spell.dbc.csv
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
| `mithril mod init` | Extract baseline DBCs from client MPQs |
| `mithril mod create <name>` | Create a new named mod |
| `mithril mod list` | List all mods and their status |
| `mithril mod status [--mod <name>]` | Show which DBCs a mod has changed |
| `mithril mod build [--mod <name>]` | Build patch MPQs from one or all mods |

See [DBC Workflow](dbc-workflow.md) for the DBC-specific commands and workflow.

See [Addons Workflow](addons-workflow.md) for addon-specific commands and workflow.

## Supported Modding Workflows

### DBC Editing

DBC files define game data: spells, items, talents, creatures, maps, and more. See [DBC Workflow](dbc-workflow.md) for the full guide.

### Addon / UI Modding

The WoW client's built-in UI is implemented as Lua/XML addons inside the MPQ archives. Mithril extracts all 465+ addon files (`.lua`, `.xml`, `.toc`) from the client's locale MPQs to the baseline, and lets you override them per-mod. Addon modifications go into locale-specific MPQs (`patch-enUS-M.MPQ`) to ensure they have higher priority than the base UI files. See [Addon Workflow](addons-workflow.md) for the full guide.

### Future Workflows

The modding framework is designed to support additional workflows as they're implemented:

- **SQL patches** — Server-side database changes (creature stats, loot tables, quest scripts)
- **Asset replacement** — Textures, models, and other art assets
- **Map editing** — ADT/WDT terrain modifications

Each workflow will follow the same pattern: mods are isolated directories, changes are tracked against a baseline, and output is packaged for the client or server as appropriate.
