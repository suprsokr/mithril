# Core Patches Workflow

Core patches modify the TrinityCore C++ server code itself — adding new features, fixing bugs, changing core game mechanics, or implementing custom systems. Patches are standard git `.patch` or `.diff` files that are applied to the TrinityCore source tree before compilation.

## Quick Start

```bash
# 1. Create your patch (from a TrinityCore fork)
cd /path/to/my/TrinityCore-fork
git diff > my-change.patch

# 2. Place it in your mod
cp my-change.patch modules/my-mod/core-patches/

# 3. Apply to the local TrinityCore source
mithril mod core apply --mod my-mod

# 4. Rebuild the server
mithril server rebuild
```

## Directory Structure

Core patches live in a mod's `core-patches/` directory:

```
modules/my-mod/
├── mod.json
├── core-patches/
│   ├── 001_custom_spell_handler.patch
│   ├── 002_increased_stack_size.patch
│   └── 003_custom_commands.patch
├── sql/
│   └── ...
└── dbc/
    └── ...
```

Patches are applied in filename order. Use numbered prefixes (`001_`, `002_`) to control the sequence.

## Creating Patches

### From Uncommitted Changes

```bash
cd /path/to/TrinityCore
# Make your changes...
git diff > my-change.patch
```

### From Committed Changes

```bash
# Single commit
git format-patch -1 HEAD --stdout > my-change.patch

# Multiple commits
git format-patch HEAD~3 --stdout > my-changes.patch
```

### From a Branch

```bash
git diff main..my-feature-branch > my-feature.patch
```

## Commands

### List Source Patches

```bash
# List all core patches across all mods
mithril mod core list

# List patches for a specific mod
mithril mod core list --mod my-mod
```

Shows each patch with its applied/pending status.

### Apply Patches

```bash
# Apply pending patches for one mod
mithril mod core apply --mod my-mod

# Apply pending patches for all mods
mithril mod core apply
```

Patches are applied using `git apply` against the TrinityCore source tree at `mithril-data/TrinityCore/`. If a patch doesn't apply cleanly, application stops to prevent partial changes.

### Check Status

```bash
mithril mod core status [--mod <name>]
```

Shows applied/pending status for each patch.

## After Applying

Core patches modify the TrinityCore C++ code, which must be recompiled for changes to take effect. After applying patches:

```bash
# Recompile TrinityCore (incremental)
mithril server rebuild

# Restart the server with the new build
mithril server restart
```

## Patch Tracking

Applied patches are tracked in `modules/core_patches_applied.json`. The tracker records the mod name, filename, and timestamp for each applied patch. Already-applied patches are skipped on subsequent runs.

## Common Patch Targets

### Custom Spell Handlers

Modify spell behavior beyond what DBC changes can achieve:

```
src/server/game/Spells/SpellEffects.cpp
src/server/game/Spells/SpellScript.cpp
```

### Custom Chat Commands

Add GM or player commands:

```
src/server/game/Chat/ChatCommands/
src/server/scripts/Commands/
```

### Core Mechanics

Adjust fundamental game systems:

```
src/server/game/Entities/Player/Player.cpp    # Player mechanics
src/server/game/Entities/Unit/Unit.cpp        # Combat formulas
src/server/game/Loot/LootMgr.cpp             # Loot system
src/server/game/World/World.cpp               # World settings
```

### Custom Scripts

Add new creature/instance/spell scripts:

```
src/server/scripts/Custom/
```

## Tips

- **Name patches descriptively** — use the `NNN_description.patch` convention for ordering and clarity
- **Keep patches small and focused** — one logical change per patch makes debugging easier
- **Test patches against a clean TrinityCore checkout** before distributing
- **Core patches require a full rebuild** — this can take 10-30 minutes depending on hardware
- **Patches may break on TrinityCore updates** — when updating TrinityCore, check that your patches still apply cleanly
- The TrinityCore source is at `mithril-data/TrinityCore/` — you can inspect it directly
- Use `mithril mod status` to see core patch status alongside other mod changes
- Core patches complement SQL migrations — patches change the C++ engine, SQL changes the database content

## Combining with Other Mod Types

A complete custom feature often requires changes at multiple levels:

| Layer | Mod Type | Example |
|---|---|---|
| Client display | DBC edit | Add spell to spell book |
| Client UI | Addon edit | Custom UI for the spell |
| Server logic | Core patch | Implement the spell's C++ handler |
| Server data | SQL migration | Configure spell parameters in the database |
| Client binary | Binary patch | Enable required client features |

All of these can live in a single mod, making the entire feature self-contained and distributable.
