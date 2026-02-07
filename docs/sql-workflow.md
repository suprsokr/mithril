# SQL Migration Workflow

SQL migrations modify databases used by the TrinityCore server and the DBC editing system. Mithril provides a numbered migration system with forward and rollback files, tracking which migrations have been applied.

## Quick Start

```bash
# 1. Create a migration (generates forward + rollback pair)
mithril mod sql create add_custom_npc --mod my-mod

# 2. Edit the generated SQL files
$EDITOR modules/my-mod/sql/world/001_add_custom_npc.sql
$EDITOR modules/my-mod/sql/world/001_add_custom_npc.rollback.sql

# 3. Apply it (server must be running)
mithril mod sql apply --mod my-mod
```

## Databases

| Database | Description | Common Use |
|---|---|---|
| `world` | Game world data (default) | Creatures, items, quests, spells, loot, spawns |
| `characters` | Character save data | Character modifications, inventory adjustments |
| `auth` | Authentication and accounts | Realm settings, account data |
| `dbc` | DBC table data (imported from client) | Bulk DBC editing via SQL (see [DBC Workflow](dbc-workflow.md)) |

Specify the database when creating a migration:

```bash
mithril mod sql create set_xp_rate --mod my-mod                  # defaults to "world"
mithril mod sql create add_realm --mod my-mod --db auth           # auth database
mithril mod sql create enable_flying --mod my-mod --db dbc        # dbc database
```

## Directory Structure

Migrations are organized by database under a mod's `sql/` directory. Each migration is a pair: a forward file and a rollback file.

```
modules/my-mod/
├── mod.json
└── sql/
    ├── world/
    │   ├── 001_add_custom_npc.sql
    │   ├── 001_add_custom_npc.rollback.sql
    │   ├── 002_adjust_loot_table.sql
    │   └── 002_adjust_loot_table.rollback.sql
    ├── dbc/
    │   ├── 001_enable_flying.sql
    │   └── 001_enable_flying.rollback.sql
    ├── auth/
    │   ├── 001_add_realm.sql
    │   └── 001_add_realm.rollback.sql
    └── characters/
        ├── 001_reset_cooldowns.sql
        └── 001_reset_cooldowns.rollback.sql
```

Migrations without a database subdirectory (placed directly in `sql/`) default to the `world` database.

## Commands

### Create a Migration

```bash
mithril mod sql create <name> --mod <mod_name> [--db <database>]
```

Generates a numbered migration pair:
- `NNN_name.sql` — forward migration (applied by `sql apply` and `mod build`)
- `NNN_name.rollback.sql` — rollback migration (applied by `sql rollback`)

The number is auto-incremented based on existing migrations for that database.

### List Migrations

```bash
# List all migrations across all mods
mithril mod sql list

# List migrations for a specific mod
mithril mod sql list --mod my-mod
```

Shows each migration with its applied/pending status.

### Apply Migrations

```bash
# Apply pending migrations for one mod
mithril mod sql apply --mod my-mod

# Apply pending migrations for all mods
mithril mod sql apply
```

Migrations are applied in filename order (which is numeric order). If a migration fails, execution stops immediately to prevent out-of-order application. Rollback files are never auto-applied.

Server SQL migrations (world/auth/characters) require the server to be running — they execute via `docker exec` against the MySQL instance inside the container. DBC migrations connect directly to MySQL on port 3306.

### Rollback Migrations

```bash
# Roll back the most recent applied migration for a mod
mithril mod sql rollback --mod my-mod

# Roll back a specific migration
mithril mod sql rollback --mod my-mod 001_add_custom_npc

# Roll back and immediately re-apply the forward migration
mithril mod sql rollback --mod my-mod --reapply
mithril mod sql rollback --mod my-mod 001_add_custom_npc --reapply
```

Rollback runs the `.rollback.sql` file and removes the migration from the tracker. With `--reapply`, it immediately re-runs the forward `.sql` file and re-adds it to the tracker.

### Check Status

```bash
mithril mod sql status [--mod <name>]
```

Same as `list` — shows applied/pending status for each migration.

## Development Loop

The typical iteration cycle when developing a migration:

```bash
# Create and write the migration
mithril mod sql create my_change --mod my-mod --db dbc
# Edit both files:
#   sql/dbc/001_my_change.sql          ← forward
#   sql/dbc/001_my_change.rollback.sql ← rollback

# Apply and test
mithril mod sql apply --mod my-mod
mithril mod build                  # for DBC migrations: exports + packages
mithril server restart

# Need to change the migration? Rollback → edit → reapply
mithril mod sql rollback --mod my-mod --reapply
mithril mod build
mithril server restart
```

For DBC migrations, `mod build` automatically applies pending migrations and exports the modified tables — so you can skip the explicit `sql apply` step.

## Migration Tracking

Applied migrations are tracked in `modules/sql_migrations_applied.json`. This file records the mod name, filename, database, and timestamp for each applied migration. Migrations are only executed once — re-running `mithril mod sql apply` skips already-applied migrations.

Rollback removes entries from this tracker, allowing them to be re-applied.

## Example Migrations

### Add a Custom NPC (world)

**Forward** (`001_add_custom_npc.sql`):

```sql
-- Create a custom vendor NPC
INSERT INTO creature_template (entry, name, subname, modelid1, minlevel, maxlevel, faction, npcflag)
VALUES (100000, 'Mithril Vendor', 'Custom Items', 26499, 80, 80, 35, 128);

-- Spawn in Stormwind
INSERT INTO creature (guid, id1, map, position_x, position_y, position_z, orientation)
VALUES (900000, 100000, 0, -8835.0, 623.0, 94.0, 3.7);
```

**Rollback** (`001_add_custom_npc.rollback.sql`):

```sql
DELETE FROM creature WHERE guid = 900000;
DELETE FROM creature_template WHERE entry = 100000;
```

### Enable Flying in Azeroth (dbc)

**Forward** (`001_enable_flying.sql`):

```sql
UPDATE areatable
SET flags = (flags | 1024) & ~536870912
WHERE map_id IN (0, 1);
```

**Rollback** (`001_enable_flying.rollback.sql`):

```sql
UPDATE areatable
SET flags = (flags & ~1024) | 536870912
WHERE map_id IN (0, 1);
```

### Adjust XP Rates (world)

**Forward** (`001_double_xp.sql`):

```sql
UPDATE quest_template SET RewardXPDifficulty = RewardXPDifficulty * 2
WHERE RewardXPDifficulty > 0;
```

**Rollback** (`001_double_xp.rollback.sql`):

```sql
UPDATE quest_template SET RewardXPDifficulty = RewardXPDifficulty / 2
WHERE RewardXPDifficulty > 0;
```

## Tips

- **Always write the rollback** when you write the forward migration — it's much easier when the logic is fresh
- **Always test migrations on a fresh database first** before applying to a server with player data
- **Use high entry IDs** (100000+) for custom content to avoid conflicts with TrinityCore updates
- **The server must be running** for `sql apply` on world/auth/characters databases
- Some changes take effect immediately (creature spawns), others require a server restart (template changes)
- DBC migrations are automatically applied during `mod build` — you don't need to run `sql apply` separately
- Use `--reapply` to iterate quickly: edit the SQL, rollback+reapply in one command
- Migrations run in filename order — use the `NNN_` prefix to control execution sequence
