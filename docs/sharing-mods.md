# Sharing Mods

Mithril mods can be shared with the community through the **Mithril Mod Registry** — a GitHub repository where mod authors register their mods and link to their source repositories.

The primary distribution model is **git clone + build locally**. When a mithril user installs a mod, they get the full source (SQL migrations, addon files, core patches, etc.) and build it with `mithril mod build`. This means every user has the source and can inspect, customize, or fork the mod.

For mod authors who also want to support non-mithril users, there's an optional `publish export` command that creates pre-built `client.zip` and `server.zip` release artifacts.

## Registry

The registry lives at [github.com/suprsokr/mithril-registry](https://github.com/suprsokr/mithril-registry). Each mod is a JSON file in the `mods/` directory containing metadata and a link to the mod's git repo.

## Discovering Mods

### List All Mods

```bash
mithril mod registry list
```

### Search

```bash
mithril mod registry search "flying"
mithril mod registry search "pvp"
mithril mod registry search "ui"
```

Searches mod names, descriptions, tags, authors, and mod types.

### Get Details

```bash
mithril mod registry info fly-in-azeroth
```

Shows full metadata including description, author, version, repo URL, and mod types.

## Installing Mods

```bash
mithril mod registry install fly-in-azeroth
```

This will:

1. Clone the mod's git repository into `modules/<mod-name>/`
2. Create a `mod.json` with an assigned patch slot (if one doesn't exist)
3. Print next steps based on the mod's content types

After installing, follow the mod's README for any setup steps, then use the standard mod commands:

```bash
mithril mod build --mod fly-in-azeroth       # Build client patches
mithril mod sql apply --mod fly-in-azeroth   # Apply SQL migrations (if any)
mithril server restart                        # Restart for changes to take effect
```

## Publishing Your Mod

### 1. Push to Git

Push your mod's directory to a git repository (e.g. GitHub). The repository should mirror the mithril mod layout:

```
my-mod/
├── mod.json
├── addons/
│   └── Interface/FrameXML/SpellBookFrame.lua
├── binary-patches/
│   └── allow-custom-gluexml.json
├── sql/
│   └── world/
│       └── 001_add_custom_npc.sql
├── core-patches/
│   └── 001_custom_handler.patch
└── README.md
```

When someone runs `mithril mod registry install my-mod`, the repository is cloned directly into `modules/my-mod/`, and all the standard mod commands work immediately.

### 2. Register in the Registry

```bash
mithril mod publish register --mod my-mod --repo https://github.com/your-username/my-mod
```

This generates a `my-mod.registry.json` file. Then:

1. Edit the JSON to fill in `author`, `description`, and `tags`
2. Fork [github.com/suprsokr/mithril-registry](https://github.com/suprsokr/mithril-registry)
3. Copy the JSON file to `mods/my-mod.json` in your fork
4. Submit a pull request

### Registry JSON Format

```json
{
  "name": "my-mod",
  "description": "A short description of what the mod does",
  "author": "github-username",
  "repo": "https://github.com/username/my-mod",
  "tags": ["spells", "fun"],
  "version": "1.0.0",
  "mod_types": ["dbc", "sql", "addon"]
}
```

See the [registry README](https://github.com/suprsokr/mithril-registry) for the full schema and guidelines.

## Optional: Pre-Built Release Artifacts

If you want to support users who don't use mithril (e.g. people who manually manage their WoW client and server files), you can export pre-built release artifacts:

```bash
mithril mod build --mod my-mod
mithril mod publish export --mod my-mod
```

This creates two zip files in `modules/build/release/my-mod/`:

- **`client.zip`** — Client-side files:
  - `Data/patch-<slot>.MPQ` — DBC patches
  - `Data/enUS/patch-enUS-<slot>.MPQ` — Addon patches
  - `binary-patches/*.json` — Binary patch descriptors

- **`server.zip`** — Server-side files:
  - `sql/<database>/*.sql` — SQL migrations
  - `core-patches/*.patch` — TrinityCore patches
  - `dbc/*.dbc` — DBC files for the server

Upload these to a GitHub release so non-mithril users can download them. **`Wow.exe` is never included** in release artifacts.

This is purely a compatibility feature — mithril users always install from the git repo and build locally.

## Tips

- **Test your mod thoroughly** before publishing — broken mods erode community trust
- **Use semantic versioning** (1.0.0, 1.1.0, etc.) for your releases
- **Tag your mod** with relevant categories so others can find it
- **Include a README** in your mod's repository explaining what it does and how to use it
- **Never include Wow.exe** in release artifacts
- Mods installed via `registry install` work exactly like locally created mods
