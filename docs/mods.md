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

- **Per-mod MPQs** (`patch-A.MPQ`, `patch-B.MPQ`, ...) in `modules/build/` for individual testing
- **Combined `patch-M.MPQ`** merging all mods, deployed to `client/Data/`

All these sort after `patch-3.MPQ` (letters sort above numbers in the patch chain), ensuring mod changes take priority over the base game. Slot M is reserved for the combined build, so it always loads last.

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
        ├── patch-A.MPQ             # Per-mod MPQ (my-spell-mod, slot A)
        ├── patch-B.MPQ             # Per-mod MPQ (my-item-mod, slot B)
        └── patch-M.MPQ             # Combined MPQ (all mods merged)
```

## Commands

| Command | Description |
|---|---|
| `mithril mod init` | Extract baseline DBCs from client MPQs |
| `mithril mod create <name>` | Create a new named mod |
| `mithril mod list` | List all mods and their status |
| `mithril mod status [--mod <name>]` | Show which DBCs a mod has changed |
| `mithril mod build [--mod <name>]` | Build `patch-M.MPQ` from one or all mods |

See [DBC Workflow](dbc-workflow.md) for the DBC-specific commands and workflow.

## Supported Modding Workflows

### DBC Editing

The primary modding workflow today. DBC files define game data: spells, items, talents, creatures, maps, and more. See [DBC Workflow](dbc-workflow.md) for the full guide.

### Future Workflows

The modding framework is designed to support additional workflows as they're implemented:

- **SQL patches** — Server-side database changes (creature stats, loot tables, quest scripts)
- **Lua scripts** — Custom UI addons and server-side scripting
- **Asset replacement** — Textures, models, and other art assets
- **Map editing** — ADT/WDT terrain modifications

Each workflow will follow the same pattern: mods are isolated directories, changes are tracked against a baseline, and output is packaged for the client or server as appropriate.
