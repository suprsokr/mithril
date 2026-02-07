# SQL Migration Workflow

SQL migrations modify the TrinityCore server databases — changing creature stats, loot tables, quest logic, NPC spawns, and more. Mithril provides a simple numbered migration system that tracks which SQL files have been applied, preventing duplicate execution.

## Quick Start

```bash
# 1. Create a migration
mithril mod sql create add_custom_npc --mod my-mod

# 2. Edit the generated SQL file
$EDITOR modules/my-mod/sql/world/001_add_custom_npc.sql

# 3. Apply it (server must be running)
mithril mod sql apply --mod my-mod
```

## Databases

TrinityCore uses three databases:

| Database | Description | Common Use |
|---|---|---|
| `world` | Game world data (default) | Creatures, items, quests, spells, loot, spawns |
| `characters` | Character save data | Character modifications, inventory adjustments |
| `auth` | Authentication and accounts | Realm settings, account data |

Specify the database when creating a migration:

```bash
mithril mod sql create set_xp_rate --mod my-mod                 # defaults to "world"
mithril mod sql create add_realm --mod my-mod --db auth          # auth database
mithril mod sql create reset_cooldowns --mod my-mod --db characters
```

## Directory Structure

Migrations are organized by database under a mod's `sql/` directory:

```
modules/my-mod/
├── mod.json
├── sql/
│   ├── world/
│   │   ├── 001_add_custom_npc.sql
│   │   ├── 002_adjust_loot_table.sql
│   │   └── 003_custom_spell_script.sql
│   ├── auth/
│   │   └── 001_add_realm.sql
│   └── characters/
│       └── 001_reset_cooldowns.sql
└── dbc/
    └── ...
```

Migrations without a database subdirectory (placed directly in `sql/`) default to the `world` database.

## Commands

### Create a Migration

```bash
mithril mod sql create <name> --mod <mod_name> [--db <database>]
```

Generates a numbered SQL file with a template header. The number is auto-incremented based on existing migrations for that database.

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

Migrations are applied in filename order (which is numeric order). If a migration fails, execution stops immediately to prevent out-of-order application.

**The server must be running** — migrations are executed via `docker exec` against the MySQL instance inside the container.

### Check Status

```bash
mithril mod sql status [--mod <name>]
```

Same as `list` — shows applied/pending status for each migration.

## Migration Tracking

Applied migrations are tracked in `modules/sql_migrations_applied.json`. This file records the mod name, filename, database, and timestamp for each applied migration. Migrations are only executed once — re-running `mithril mod sql apply` skips already-applied migrations.

## Example Migrations

### Add a Custom NPC

```sql
-- Migration: add_custom_npc
-- Database: world

-- Create a custom vendor NPC
INSERT INTO creature_template (entry, name, subname, modelid1, minlevel, maxlevel, faction, npcflag)
VALUES (100000, 'Mithril Vendor', 'Custom Items', 26499, 80, 80, 35, 128);

-- Spawn in Stormwind
INSERT INTO creature (guid, id1, map, position_x, position_y, position_z, orientation)
VALUES (900000, 100000, 0, -8835.0, 623.0, 94.0, 3.7);
```

### Adjust XP Rates

```sql
-- Migration: set_xp_rate
-- Database: world

-- Double quest XP rewards
UPDATE quest_template SET RewardXPDifficulty = RewardXPDifficulty * 2
WHERE RewardXPDifficulty > 0;
```

### Custom Loot Table

```sql
-- Migration: custom_loot
-- Database: world

-- Add a custom item drop to Onyxia
INSERT INTO creature_loot_template (Entry, Item, Chance, LootMode, GroupId, MinCount, MaxCount)
VALUES (10184, 100001, 50, 1, 0, 1, 1);
```

## Tips

- **Always test migrations on a fresh database first** before applying to a server with player data
- **Migrations are one-way** — there's no automatic rollback. Write a separate "undo" migration if needed
- **Use high entry IDs** (100000+) for custom content to avoid conflicts with TrinityCore updates
- **The server must be running** for `sql apply` — it executes SQL through the Docker container
- Some changes take effect immediately (creature spawns), others require a server restart (template changes)
- Use `mithril mod status` to see SQL migration status alongside DBC and addon changes
- Migrations run in filename order — use the `NNN_` prefix to control execution sequence
