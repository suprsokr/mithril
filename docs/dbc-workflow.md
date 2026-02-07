# DBC Modding Workflow

DBC (DataBase Client) files define the core game data for WoW 3.3.5a — spells, items, talents, creature display info, maps, and much more. Mithril extracts these from the client's MPQ archives, imports them into MySQL tables for editing with SQL, and repackages modifications back into a patch MPQ.

## Quick Start

```bash
# 1. Extract baseline + import into MySQL
mithril mod init

# 2. Create a mod
mithril mod create my-mod

# 3. Explore the data
mithril mod dbc query "SELECT id, name_enus, flags FROM areatable WHERE map_id = 0 LIMIT 5"

# 4. Create a DBC SQL migration
mithril mod sql create enable_flying --mod my-mod --db dbc
# Edit the generated file with your SQL

# 5. Build the patch (applies migration, exports DBC, packages MPQ)
mithril mod build --mod my-mod

# 6. Restart the server
mithril server restart
```

## Step-by-Step

### 1. Extract Baseline

```bash
mithril mod init
```

This scans the client's `Data/` folder, opens all MPQ archives in the correct patch chain order, and extracts every DBC file. The 97 DBCs with known schemas are imported into MySQL tables in the `dbc` database.

Output:
- `modules/baseline/dbc/` — raw `.dbc` binaries
- `modules/baseline/manifest.json` — extraction metadata
- MySQL `dbc` database — all 97 DBC tables (e.g., `areatable`, `spell`, `map`)

> **Note:** The server must be running for the MySQL import step. If it isn't, `mod init` will warn and you can import later with `mithril mod dbc import`.

### 2. Create a Mod

```bash
mithril mod create my-spell-mod
```

This creates `modules/my-spell-mod/` with a `mod.json` and assigns a **patch slot** (A, B, C, ... L, AA, AB, ...) that determines the MPQ filename (e.g., `patch-A.MPQ`). Slot M is reserved for the combined build.

### 3. Explore DBCs

**Query with SQL (recommended):**

```bash
# See what tables are available
mithril mod dbc query "SHOW TABLES"

# Inspect a table's schema
mithril mod dbc query "DESCRIBE spell"

# Search for data
mithril mod dbc query "SELECT id, spell_name_enus, effect_1 FROM spell WHERE spell_name_enus LIKE '%Fireball%'"

# Complex queries with joins, aggregation, etc.
mithril mod dbc query "SELECT map_id, COUNT(*) as zones FROM areatable GROUP BY map_id ORDER BY zones DESC"
```

**Other exploration commands:**

```bash
mithril mod dbc list                  # List all 97 DBCs with record/field counts
mithril mod dbc search "Fireball"     # Search across all DBC tables
mithril mod dbc inspect Spell         # Show schema, field types, and sample records
```

### 4. Edit a DBC

#### SQL Migrations (recommended)

SQL migrations are the best approach for DBC editing — they're expressive, composable, and handle bulk operations naturally.

**Create a migration:**

```bash
mithril mod sql create enable_flying --mod my-mod --db dbc
```

This creates a migration pair in `modules/my-mod/sql/dbc/`:
- `001_enable_flying.sql` — forward migration
- `001_enable_flying.rollback.sql` — rollback migration

Edit both files with your changes:

```sql
-- Enable flying in Eastern Kingdoms and Kalimdor
UPDATE areatable
SET flags = (flags | 1024) & ~536870912
WHERE map_id IN (0, 1);
```

Any valid SQL works — UPDATE, INSERT, DELETE, ALTER, subqueries, joins:

```sql
-- Set all Mage spells to zero mana cost
UPDATE spell
SET mana_cost = 0
WHERE id IN (
    SELECT spell FROM skilllineability
    WHERE skillline IN (
        SELECT id FROM skillline WHERE name_enus = 'Frost'
    )
);
```

```sql
-- Add a custom spell (high ID to avoid conflicts)
INSERT INTO spell (id, spell_name_enus, school, mana_cost, effect_1)
VALUES (100001, 'Mithril Bolt', 4, 50, 2);
```

Migrations are tracked — each forward migration runs only once, even across multiple builds. Rollback files are never auto-applied.

**Iterating on a migration:**

```bash
# Edit your .sql file, then rollback and re-apply in one command:
mithril mod sql rollback --mod my-mod --reapply
mithril mod build --mod my-mod
```

This runs the `.rollback.sql` to undo the previous version, then re-applies the updated `.sql` file. See [SQL Workflow](sql-workflow.md) for details.


### 5. Check Status

```bash
# Status for one mod
mithril mod status --mod my-mod

# Status for all mods
mithril mod status
```

Shows which DBCs each mod has modified and which SQL migrations are pending.

### 6. Build the Patch

```bash
# Build one mod
mithril mod build --mod my-mod

# Build all mods together
mithril mod build
```

The build process:
1. Applies pending DBC SQL migrations (from `sql/dbc/`) against the MySQL `dbc` database
2. Determines which tables were touched by the SQL and exports them back to binary `.dbc` format
3. Creates a **per-mod DBC MPQ** in `modules/build/` (e.g., `patch-A.MPQ`)
4. If the mod also has addon changes, creates a **per-mod addon MPQ** (e.g., `patch-enUS-A.MPQ`)
5. Creates combined MPQs (`patch-M.MPQ`, `patch-enUS-M.MPQ`) when building all mods
6. Deploys DBC MPQ to `client/Data/`, addon MPQ to `client/Data/<locale>/`
7. Copies modified `.dbc` files to the **server's `data/dbc/`** directory
8. Cleans any previous mithril patches from the client before deploying

### Client vs. Server

DBC files are used by **both** the WoW client and the TrinityCore server:

- **Client** reads DBCs from MPQ archives (patch chain). The build produces `patch-M.MPQ` for this.
- **Server** reads DBCs from flat files on disk (`data/dbc/`). The build copies modified `.dbc` files directly into this directory.

Both are updated automatically by `mithril mod build`. After building:

- **Client changes** take effect immediately on next login (no server restart needed)
- **Server changes** require a restart: `mithril server restart`

> **Note:** Some DBC changes are purely cosmetic (spell names, icons, descriptions) and only need the client-side update. Others affect gameplay logic (spell effects, damage values, talent trees) and require both client and server to have the updated DBC.

## Multiple Mods

Mods are independent. Each mod only contains the files it modifies:

```bash
mithril mod create spell-tweaks
mithril mod create custom-talents
mithril mod create new-items

mithril mod sql create zero_mana --mod spell-tweaks --db dbc
mithril mod sql create rearrange --mod custom-talents --db dbc
```

When building all mods together (`mithril mod build`), files from all mods are combined into a single `patch-M.MPQ`. DBC SQL migrations are applied in mod-alphabetical order. SQL changes stack since they all modify the same database.

## Managing the DBC Database

```bash
# Re-import baseline (resets all SQL changes)
mithril mod dbc import --force

# Run ad-hoc queries
mithril mod dbc query "SELECT COUNT(*) FROM spell"

# Export all modified tables to .dbc files
mithril mod dbc export

# Roll back a specific migration
mithril mod sql rollback --mod my-mod 001_enable_flying

# Roll back and re-apply (for iterating on a migration)
mithril mod sql rollback --mod my-mod --reapply
```

Re-importing with `--force` resets the `dbc` database to pristine baseline state. All DBC migrations will be re-applied on the next build. Use `sql rollback --reapply` for quick iteration without a full reimport.

## Tips

- **Always work in a mod**, never edit `modules/baseline/` directly
- **Always write the rollback** when you write the forward migration — it's much easier when the logic is fresh
- Use SQL for anything involving more than a single row — it's faster and less error-prone
- Use `mithril mod dbc query "DESCRIBE tablename"` to see column names and types
- Use high IDs (100000+) for custom content to avoid conflicts with base game data
- Use `--reapply` to iterate quickly: edit the SQL, rollback+reapply in one command
- All SQL migrations (world, auth, characters, dbc) use the same tracker and rollback system
- After building, restart the WoW client to see client-side changes
- After building, run `mithril server restart` for server-side changes to take effect
- Cosmetic changes (names, icons) only need a client restart; gameplay changes (damage, effects) need both
