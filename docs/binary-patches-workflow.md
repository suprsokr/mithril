# Binary Patches

Binary patches modify the WoW client executable (`Wow.exe`) at specific byte offsets to change client behavior. Mithril includes built-in patches for common needs and supports custom patches defined as JSON files.

## Quick Start

```bash
# List available patches
mithril mod patch list

# Apply a built-in patch
mithril mod patch apply allow-custom-gluexml

# Check what's applied
mithril mod patch status

# Restore original Wow.exe
mithril mod patch restore
```

## Built-in Patches

| Patch | Description |
|---|---|
| `allow-custom-gluexml` | **Required for addon modding.** Disables the client's GlueXML/FrameXML integrity check, preventing the "corrupt interface files" crash when modifying built-in UI files. |
| `large-address-aware` | Enables the Large Address Aware flag, allowing the client to use more than 2GB of RAM on 64-bit systems. |

## Commands

### List Available Patches

```bash
mithril mod patch list
```

Shows all built-in patches and any custom patches found in your mods' `binary-patches/` directories.

### Apply Patches

```bash
# Apply a built-in patch by name
mithril mod patch apply allow-custom-gluexml

# Apply multiple patches at once
mithril mod patch apply allow-custom-gluexml large-address-aware

# Apply a custom patch from a JSON file
mithril mod patch apply modules/my-mod/binary-patches/custom.json
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

## Build Integration

When `mithril mod build` detects that a mod includes GlueXML or FrameXML addon changes, it checks whether `allow-custom-gluexml` has been applied. If not, it displays a warning:

```
⚠ Your mod includes GlueXML/FrameXML changes. The client will crash
  with 'corrupt interface files' unless you apply the binary patch:
  mithril mod patch apply allow-custom-gluexml
```

## Important Notes

- **Patches are designed for the clean WoW 3.3.5a (build 12340) client.** Using a different version may cause crashes or unexpected behavior.
- **Always keep your `Wow.exe.clean` backup safe.** If you lose it, you'll need a fresh copy of the original client.
- **Binary patches are client-only** — they don't affect the server.
- **Patches persist across builds** — you only need to apply them once. They survive `mithril mod build` since the build only modifies MPQ files, not `Wow.exe`.

## See Also

- [Mods Overview](mods.md) — General modding framework
- [Addon Workflow](addons-workflow.md) — Addon modding (requires `allow-custom-gluexml`)
- [DBC Workflow](dbc-workflow.md) — DBC modding
