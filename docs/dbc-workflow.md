# DBC Modding Workflow

DBC (DataBase Client) files define the core game data for WoW 3.3.5a — spells, items, talents, creature display info, maps, and much more. Mithril extracts these from the client's MPQ archives, converts them to editable CSV files with named columns, and can repackage modifications back into a patch MPQ.

## Quick Start

```bash
# 1. Extract baseline from client
mithril mod init

# 2. Create a mod
mithril mod create my-mod

# 3. Edit a DBC
mithril mod dbc set Spell --mod my-mod --where id=133 --set spell_name_enUS="Mithril Bolt"

# 4. Build the patch
mithril mod build --mod my-mod

# 5. Launch the client — your changes are live
```

## Step-by-Step

### 1. Extract Baseline

```bash
mithril mod init
```

This scans the client's `Data/` folder, opens all MPQ archives in the correct patch chain order, and extracts every DBC file. For the 97 DBCs with known schemas, it also exports them to CSV with named columns. The result is stored in `modules/baseline/` and is never modified.

Output:
- `modules/baseline/dbc/` — raw `.dbc` binaries
- `modules/baseline/csv/` — pristine CSV exports (97 files)
- `modules/baseline/manifest.json` — extraction metadata

### 2. Create a Mod

```bash
mithril mod create my-spell-mod
```

This creates `modules/my-spell-mod/` with a `mod.json` and an empty `dbc/` directory. The mod starts with no changes. Each mod is automatically assigned a **patch slot** (A, B, C, ... L, AA, AB, ...) that determines its per-mod MPQ filename (e.g., `patch-A.MPQ`). Slot M is reserved for the combined build.

### 3. Explore DBCs

**List all available DBCs:**

```bash
mithril mod dbc list
```

Shows all 97 DBCs with known schemas, their record counts, and field counts.

**Search across all DBCs:**

```bash
mithril mod dbc search "Fireball"
```

Regex search across every CSV. Shows matching rows with file names and row numbers.

**Inspect a DBC's schema:**

```bash
mithril mod dbc inspect Spell
```

Shows the full schema (field names, types, array sizes), CSV column list, and sample records.

### 4. Edit a DBC

There are two ways to edit:

**Programmatic editing** (best for scripts and AI):

```bash
mithril mod dbc set Spell --mod my-mod --where id=133 --set spell_name_enUS="Mithril Bolt"
```

This finds the row where `id=133`, changes the `spell_name_enUS` column, and saves. Multiple `--set` flags can be used to change several columns at once:

```bash
mithril mod dbc set Spell --mod my-mod \
  --where id=133 \
  --set spell_name_enUS="Mithril Bolt" \
  --set spell_name_deDE="Mithrilblitz" \
  --set power_cost=50
```

**Interactive editing** (open in your editor):

```bash
mithril mod dbc edit Spell --mod my-mod
```

Opens the CSV in `$EDITOR` (or auto-detects VS Code, vim, nano). The CSV has named columns so it's easy to find what you need.

Both methods automatically copy the baseline CSV into the mod on first edit.

### 5. Check Status

```bash
# Status for one mod
mithril mod status --mod my-mod

# Status for all mods
mithril mod status
```

Shows which DBCs each mod has modified compared to the baseline.

### 6. Build the Patch

```bash
# Build one mod
mithril mod build --mod my-mod

# Build all mods together
mithril mod build
```

The build process:
1. Compares each mod's CSVs against the baseline to find actual changes
2. Converts modified CSVs back to binary DBC format
3. Creates a **per-mod DBC MPQ** in `modules/build/` (e.g., `patch-A.MPQ`)
4. If the mod also has addon changes, creates a **per-mod addon MPQ** (e.g., `patch-enUS-A.MPQ`)
5. Creates combined MPQs (`patch-M.MPQ` for DBCs, `patch-enUS-M.MPQ` for addons) when building all mods
6. Deploys DBC MPQ to `client/Data/`, addon MPQ to `client/Data/<locale>/`
7. Copies modified `.dbc` files to the **server's `data/dbc/`** directory
8. Cleans any previous mithril patches from the client before deploying

DBC and addon files are kept in separate MPQs because the WoW client loads addon files from locale-specific archives (`Data/enUS/`) with higher priority. All letter-based patches sort after `patch-3.MPQ`, ensuring mod changes override the originals.

### Client vs. Server

DBC files are used by **both** the WoW client and the TrinityCore server:

- **Client** reads DBCs from MPQ archives (patch chain). The build produces `patch-M.MPQ` for this.
- **Server** reads DBCs from flat files on disk (`data/dbc/`). The build copies modified `.dbc` files directly into this directory.

Both are updated automatically by `mithril mod build`. After building:

- **Client changes** take effect immediately on next login (no server restart needed)
- **Server changes** require a restart: `mithril server restart`

> **Note:** Some DBC changes are purely cosmetic (spell names, icons, descriptions) and only need the client-side update. Others affect gameplay logic (spell effects, damage values, talent trees) and require both client and server to have the updated DBC.

## CSV Format

Each DBC is exported as a standard CSV with a header row of named columns. The column names come from 97 embedded schema definitions covering all major 3.3.5a DBCs.

## Multiple Mods

Mods are independent. Each mod only contains the DBC files it modifies:

```bash
mithril mod create spell-tweaks
mithril mod create custom-talents
mithril mod create new-items

# Each mod edits different DBCs
mithril mod dbc set Spell --mod spell-tweaks --where id=133 --set power_cost=0
mithril mod dbc set Talent --mod custom-talents --where id=1 --set column_index=0
```

When building all mods together (`mithril mod build`), files from all mods are combined into a single `patch-M.MPQ`. If two mods modify the same DBC, the last mod alphabetically wins (conflict resolution is not yet implemented).

## Tips

- **Always work in a mod**, never edit `modules/baseline/` directly
- Use `mithril mod dbc search` to find what you want to change — it's regex-capable
- Use `mithril mod dbc inspect` to see the full schema before editing
- The `--where` flag in `set` matches exact values — use the `id` column for precision
- After building, restart the WoW client to see client-side changes
- After building, run `mithril server restart` for server-side changes to take effect
- Cosmetic changes (names, icons) only need a client restart; gameplay changes (damage, effects) need both
