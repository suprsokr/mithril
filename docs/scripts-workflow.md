# Scripts Workflow

Scripts are custom C++ source files that get compiled directly into the TrinityCore server. Unlike core patches (which modify existing TrinityCore code via `.patch` files), scripts are standalone `.cpp`/`.h` files that add new functionality — custom NPCs, spell handlers, chat commands, player event hooks, and more.

## Quick Start

```bash
mithril mod script create my_custom_npc --mod my-mod
# Edit modules/my-mod/scripts/my_custom_npc.cpp
mithril mod build
mithril server restart
```

`mod build` syncs changed scripts to the container, recompiles TrinityCore (incremental), and applies any SQL migrations.

## Script Types

Use `--type` to generate a template for a specific script type:

```bash
mithril mod script create <name> --mod <mod> --type <type>
```

| Type | Base Class | Use Case |
|------|-----------|----------|
| `creature` | `CreatureScript` | Custom NPC AI — combat logic, gossip menus, waypoints (default) |
| `player` | `PlayerScript` | Player event hooks — login, kill, chat, level up, zone change |
| `spell` | `SpellScript` / `AuraScript` | Custom spell and aura handlers |
| `command` | `CommandScript` | Custom GM / chat commands |
| `worldscript` | `WorldScript` | Server-wide event hooks — startup, shutdown, world tick |
| `item` | `ItemScript` | Item use, quest accept, expire, and removal handlers |
| `gameobject` | `GameObjectScript` | GameObject AI — interaction, periodic updates |
| `areatrigger` | `AreaTriggerScript` | Area trigger handlers — triggered when a player enters an area |
| `unit` | `UnitScript` | Unit damage and healing modifiers |

### Examples

```bash
# Custom NPC with combat AI
mithril mod script create boss_handler --mod my-mod --type creature

# Welcome message on login
mithril mod script create welcome_msg --mod my-mod --type player

# Custom spell effect
mithril mod script create super_fireball --mod my-mod --type spell

# GM command: .hello
mithril mod script create hello_cmd --mod my-mod --type command

# Server startup hook
mithril mod script create startup_hook --mod my-mod --type worldscript

# Custom item use handler
mithril mod script create magic_sword --mod my-mod --type item

# Interactive game object
mithril mod script create portal_gate --mod my-mod --type gameobject

# Area trigger for a custom zone
mithril mod script create dungeon_entrance --mod my-mod --type areatrigger

# Global damage modifier
mithril mod script create damage_tuning --mod my-mod --type unit
```

## Directory Structure

Scripts live in a mod's `scripts/` directory:

```
modules/my-mod/
├── mod.json
├── scripts/
│   ├── boss_handler.cpp
│   ├── welcome_msg.cpp
│   └── hello_cmd.cpp
├── sql/
│   └── ...
└── core-patches/
    └── ...
```

## Commands

### Create a Script

```bash
mithril mod script create <name> --mod <mod_name> [--type <type>]
```

Creates a `.cpp` file with a starter template for the specified script type. If `--type` is omitted, defaults to `creature`.

### List Scripts

```bash
# List all scripts across all mods
mithril mod script list

# List scripts for a specific mod
mithril mod script list --mod my-mod
```

### Remove a Script

```bash
mithril mod script remove <name> --mod <mod_name>
```

Removes the script file. If the server is running, you'll be prompted to rebuild to remove the script from the compiled server.

## How It Works

`mithril mod build` compares each mod's `scripts/` files against `scripts_applied.json` using checksums. Only changed, new, or removed files are synced to the container. If anything changed, TrinityCore is automatically recompiled (incremental — only changed files). Pending SQL migrations are also applied.

## Scripts vs Core Patches

| | Scripts | Core Patches |
|---|---|---|
| What they do | Add new standalone C++ code | Modify existing TrinityCore C++ code |
| File format | `.cpp` / `.h` files | `.patch` / `.diff` files |
| Location | `modules/<mod>/scripts/` | `modules/<mod>/core-patches/` |
| Conflict risk | Low — independent files | Higher — patches may conflict with TC updates |
| Use when | Adding new NPCs, spells, commands, hooks | Changing core mechanics, fixing TC bugs |

## Tips

- **One `AddSC_` function per file** — TrinityCore's CMake auto-discovers scripts in the Custom directory
- **Use high entry IDs** (100000+) for custom creatures/spells to avoid conflicts with existing content
- **Pair scripts with SQL migrations** — the script handles logic, SQL sets up the database entries
- **Incremental builds are fast** — only changed files are recompiled by `make`
- **Test in-game** with `.reload` commands where possible before restarting
