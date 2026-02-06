# Mithril Server Administration

## Commands

```
mithril init                                    # Build image, compile TC, extract maps, setup DB
mithril server start                            # Start the server container
mithril server stop                             # Stop the server container
mithril server restart                          # Restart the server container
mithril server status                           # Show container status
mithril server attach                           # Attach to the worldserver console (Ctrl+P, Ctrl+Q to detach)
mithril server logs                             # Stream container logs (Ctrl+C to stop)
mithril server account create <user> <pass> [gm_level]  # Create a game account
```

## Creating Accounts

After starting the server, create accounts with:

```bash
mithril server account create admin admin       # GM level 3 (admin, default)
mithril server account create player mypass 0   # GM level 0 (regular player)
```

GM levels: `0` Player, `1` Moderator, `2` GameMaster, `3` Administrator (default).

Accounts are created directly in the auth database using SRP6 authentication — no running worldserver console required, only the container and MySQL.

## Configuration

Configs live in `mithril-data/etc/`. They are generated from TrinityCore's `.conf.dist` defaults with Mithril-specific overrides applied. Re-running `mithril init` regenerates them.

### worldserver.conf

Generated from `worldserver.conf.dist`. Mithril overrides:

| Setting | Value | Notes |
|---------|-------|-------|
| `DataDir` | `/opt/trinitycore/data` | Map/DBC/vmap/mmap data |
| `LogsDir` | `/opt/trinitycore/log` | Server log files |
| `SourceDirectory` | `/opt/trinitycore` | SQL update source path |
| `LoginDatabaseInfo` | `127.0.0.1;3306;trinity;trinity;auth` | MySQL connection |
| `WorldDatabaseInfo` | `127.0.0.1;3306;trinity;trinity;world` | MySQL connection |
| `CharacterDatabaseInfo` | `127.0.0.1;3306;trinity;trinity;characters` | MySQL connection |
| `Updates.EnableDatabases` | `15` | Auto-update all databases |
| `PlayerLimit` | `100` | Max concurrent players |
| `SOAP.IP` | `0.0.0.0` | Bind SOAP to all interfaces |

All other settings use TrinityCore defaults. Notable defaults:

- `WorldServerPort = 8085`
- `SOAP.Enabled = 0` (disabled)
- `Warden.Enabled = 0` (disabled for dev)
- `GameType = 0` (Normal)
- `Expansion = 2` (WotLK)

### authserver.conf

Generated from `authserver.conf.dist`. Mithril overrides:

| Setting | Value | Notes |
|---------|-------|-------|
| `LoginDatabaseInfo` | `127.0.0.1;3306;trinity;trinity;auth` | MySQL connection |
| `SourceDirectory` | `/opt/trinitycore` | SQL update source path |
| `LogsDir` | `/opt/trinitycore/log` | Server log files |
| `Updates.EnableDatabases` | `1` | Auto-update auth database |

Default auth port: `RealmServerPort = 3724`.

### Editing Configs

Edit the `.conf` files directly — changes take effect on `mithril server restart`. The full list of available settings is documented in the corresponding `.conf.dist` file.

## Network Ports

| Service | Port |
|---------|------|
| Auth server | 3724 |
| World server | 8085 |
| MySQL | 3306 |

## Docker

The server runs in a single container (`mithril-server`) managed by Docker Compose. MySQL, authserver, and worldserver all run inside this container.

- Image: `mithril-server:latest` (built during `mithril init`)
- Compose project: `mithril`
- Restart policy: `unless-stopped`
- Data persisted via bind mounts in `mithril-data/`
