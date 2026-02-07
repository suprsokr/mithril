# Addon / UI Modding Workflow

The WoW 3.3.5a client's entire user interface — action bars, spell book, character frame, chat, raid frames, and more — is implemented as Lua and XML addons embedded inside the client's MPQ archives. Mithril extracts these to a shared baseline and lets you override them per-mod, then packages your changes into locale-specific patch MPQs that the client loads with highest priority.

## Quick Start

```bash
# 1. Extract baseline (includes addons)
mithril mod init

# 2. Create a mod
mithril mod create my-ui-mod

# 3. Find what to edit
mithril mod addon search "SpellButton"

# 4. Edit an addon file
mithril mod addon edit Interface/FrameXML/SpellBookFrame.lua --mod my-ui-mod

# 5. Build and deploy
mithril mod build --mod my-ui-mod
```

## How It Works

### Extraction

`mithril mod init` scans all MPQ archives in the client's patch chain and extracts every `.lua`, `.xml`, and `.toc` file under the `Interface/` directory. The latest version of each file is kept (later patches override earlier ones). All 465+ files are saved to `modules/baseline/addons/` with their original directory structure preserved.

### Baseline Structure

```
modules/baseline/addons/
├── Interface/
│   ├── AddOns/
│   │   ├── Blizzard_AchievementUI/
│   │   │   ├── Blizzard_AchievementUI.lua
│   │   │   ├── Blizzard_AchievementUI.toc
│   │   │   ├── Blizzard_AchievementUI.xml
│   │   │   └── Localization.lua
│   │   ├── Blizzard_TalentUI/
│   │   ├── Blizzard_AuctionUI/
│   │   └── ... (24 Blizzard addons)
│   ├── FrameXML/
│   │   ├── SpellBookFrame.lua
│   │   ├── SpellBookFrame.xml
│   │   ├── ActionButton.lua
│   │   ├── ChatFrame.lua
│   │   ├── UnitFrame.lua
│   │   └── ... (~300 files — the core UI framework)
│   ├── GlueXML/
│   │   ├── CharacterSelect.lua
│   │   ├── AccountLogin.lua
│   │   └── ... (login/character select screens)
│   └── LCDXML/
│       └── ... (Logitech LCD display layouts)
```

### Addon Categories

| Directory | Description | Files |
|---|---|---|
| `Interface/AddOns/` | Blizzard addon modules (achievement, talent, auction, etc.) | ~100 |
| `Interface/FrameXML/` | Core UI framework (action bars, spell book, chat, unit frames) | ~300 |
| `Interface/GlueXML/` | Login screen, character select, realm list | ~60 |
| `Interface/LCDXML/` | Logitech LCD display layouts | ~2 |

## Commands

### List All Addon Files

```bash
mithril mod addon list
```

Shows all addon files grouped by directory, with file counts. Useful for discovering what's available to modify.

### Search Addon Files

```bash
# Search across all baseline addon files
mithril mod addon search "SpellButton"

# Search within a mod (checks mod overrides + baseline)
mithril mod addon search "SpellButton" --mod my-ui-mod
```

Regex search across all `.lua`, `.xml`, and `.toc` files. Shows matching lines with file paths and line numbers.

### Edit an Addon File

```bash
mithril mod addon edit Interface/FrameXML/SpellBookFrame.lua --mod my-ui-mod
```

Opens the file in your `$EDITOR`. On first edit, the file is automatically copied from the baseline into your mod's `addons/` directory. Only files that differ from the baseline are packaged during build.

## Build Output

When a mod has addon changes, the build produces a **locale-specific MPQ** separate from the DBC MPQ:

```
=== Build Complete ===
  Build artifacts (modules/build/):
    patch-A.MPQ  ← my-ui-mod DBC (4191042 bytes)
    patch-enUS-A.MPQ  ← my-ui-mod addons (12345 bytes)

  Client DBC:    Data/patch-A.MPQ (1 files)
  Client addons: Data/enUS/patch-enUS-A.MPQ (3 files)
```

- **DBC MPQ** → `client/Data/patch-<slot>.MPQ`
- **Addon MPQ** → `client/Data/enUS/patch-enUS-<slot>.MPQ`

### Why Separate MPQs?

All addon files in the base game come from locale-specific archives (`locale-enUS.MPQ`, `patch-enUS.MPQ`, etc.). These are loaded *after* non-locale archives in the patch chain. If we put modified addon files into a non-locale `patch-M.MPQ`, the base locale patches would override our changes.

By placing addon modifications in `patch-enUS-M.MPQ` inside `Data/enUS/`, they load after all base locale patches and take priority.

### Patch Chain Order (Relevant to Addons)

```
patch-enUS.MPQ          ← base locale patch
patch-enUS-2.MPQ        ← base locale patch 2
patch-enUS-3.MPQ        ← last base locale patch
patch-enUS-A.MPQ        ← mithril mod slot A (addons)
patch-enUS-B.MPQ        ← mithril mod slot B (addons)
patch-enUS-M.MPQ        ← mithril combined (addons)
```

## Common Modding Targets

### Changing Spell Book Behavior

```bash
mithril mod addon search "SPELLBOOK" --mod my-mod
mithril mod addon edit Interface/FrameXML/SpellBookFrame.lua --mod my-mod
```

### Modifying Action Bars

```bash
mithril mod addon edit Interface/FrameXML/ActionButton.lua --mod my-mod
mithril mod addon edit Interface/FrameXML/ActionBarFrame.xml --mod my-mod
```

### Customizing the Character Frame

```bash
mithril mod addon edit Interface/FrameXML/CharacterFrame.lua --mod my-mod
mithril mod addon edit Interface/FrameXML/PaperDollFrame.lua --mod my-mod
```

### Modifying the Talent UI

```bash
mithril mod addon edit Interface/AddOns/Blizzard_TalentUI/Blizzard_TalentUI.lua --mod my-mod
```

### Changing the Login Screen

```bash
mithril mod addon edit Interface/GlueXML/AccountLogin.lua --mod my-mod
mithril mod addon edit Interface/GlueXML/CharacterSelect.lua --mod my-mod
```

## Tips

- **Always work in a mod** — never edit files in `modules/baseline/addons/` directly
- Use `addon search` to find the right file — the WoW UI code is spread across hundreds of files
- The `.toc` file for each addon lists the load order of its files — edit this if you need to add new files
- Addon changes are **client-only** — no server restart needed, just restart the WoW client
- You can combine DBC and addon changes in the same mod — the build produces separate MPQs for each
- `FrameXML` is the core UI framework; `AddOns` are optional modules loaded on demand
- WoW's Lua environment is sandboxed — you can't require external modules or access the filesystem
- Use `/reload` in-game to reload the UI without restarting the client (works for most addon changes)
