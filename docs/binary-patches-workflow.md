# Binary Patches

Binary patches modify the WoW client executable (`Wow.exe`) at specific byte offsets to change client behavior. Patches are distributed as mods with JSON files in their `binary-patches/` directories.

## Quick Start

```bash
# List available patches from installed mods
mithril mod patch list

# Apply a patch from a mod
mithril mod patch apply my-mod/binary-patches/my-patch.json

# Check what's applied
mithril mod patch status

# Restore original Wow.exe
mithril mod patch restore
```

Patches are distributed as mods with JSON files in their `binary-patches/` directories. Use `mithril mod patch list` to see all available patches from your installed mods.

## Commands

### List Available Patches

```bash
mithril mod patch list
```

Shows all patches found in your installed mods' `binary-patches/` directories.

### Apply Patches

```bash
# Apply a patch from an installed mod
mithril mod patch apply my-mod/binary-patches/my-patch.json

# Apply multiple patches at once
mithril mod patch apply my-mod/binary-patches/patch-a.json my-mod/binary-patches/patch-b.json
```

On first apply, a backup of `Wow.exe` is saved as `Wow.exe.clean`. The backup is verified against the known clean WoW 3.3.5a (build 12340) MD5 hash (`45892bdedd0ad70aed4ccd22d9fb5984`).

Patches are applied by restoring from the clean backup first, then applying all tracked patches in order. This ensures a consistent state regardless of how many patches are applied or in what order.

### Check Status

```bash
mithril mod patch status
```

Shows all applied patches with their timestamps.

### Restore Original

```bash
mithril mod patch restore
```

Restores `Wow.exe` from the clean backup and clears the patch tracker. After restoring, you can re-apply patches with `mithril mod patch apply`.

## Custom Patches

### JSON Format

Create a `.json` file in your mod's `binary-patches/` directory:

```
modules/my-mod/
├── mod.json
├── dbc/
├── addons/
└── binary-patches/
    └── my-custom-patch.json
```

### Structure

```json
{
  "name": "my-custom-patch",
  "description": "What this patch does",
  "patches": [
    {
      "address": "0x415a25",
      "bytes": ["0xeb"]
    },
    {
      "address": "0x415a3f",
      "bytes": ["0x03", "0x00", "0x00"]
    }
  ]
}
```

### Fields

| Field | Description |
|---|---|
| `name` | Optional display name for the patch |
| `description` | Optional description shown in `patch list` |
| `patches` | Array of byte patches to apply |
| `patches[].address` | Hex offset in the executable (e.g., `"0x415a25"` or `"415a25"`) |
| `patches[].bytes` | Array of hex byte values to write (e.g., `["0xEB", "0x26"]`) |

## How It Works

1. **Backup** — On first apply, `Wow.exe` is copied to `Wow.exe.clean`
2. **Verify** — The backup is checked against the known clean client MD5
3. **Restore** — `Wow.exe` is restored from the clean backup (ensures clean slate)
4. **Apply** — All tracked patches are re-applied in order, then new patches are applied
5. **Track** — Applied patches are recorded in `modules/binary_patches_applied.json`

This restore-then-apply approach ensures patches never conflict with each other, regardless of application order.

## Important Notes

- **Patches are designed for the clean WoW 3.3.5a (build 12340) client.** Using a different version may cause crashes or unexpected behavior.
- **Always keep your `Wow.exe.clean` backup safe.** If you lose it, you'll need a fresh copy of the original client.
- **Binary patches are client-only** — they don't affect the server.
- **Patches persist across builds** — you only need to apply them once. They survive `mithril mod build` since the build only modifies MPQ files, not `Wow.exe`.

## See Also

- [Mods Overview](mods.md) — General modding framework
- [Addon Workflow](addons-workflow.md) — Addon modding
- [DBC Workflow](dbc-workflow.md) — DBC modding
