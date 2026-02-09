# Mithril

Integrated development environment for TrinityCore and WoW 3.3.5a modding.

## Prerequisites

- **Go 1.22+** — [go.dev/dl](https://go.dev/dl/)
- **Docker** — [docs.docker.com/get-docker](https://docs.docker.com/get-docker/)
- **WoW 3.3.5a client** (build 12340) — you must supply your own copy
- **MySQL client libraries** — needed for DBC database operations

### Platform Notes

| Platform | Notes |
|----------|-------|
| Linux    | Fully supported. Docker must be running. |
| macOS    | Fully supported. Docker Desktop or Colima required. |
| Windows  | Use WSL2 with Docker Desktop. |

## Install

```bash
git clone --recursive https://github.com/suprsokr/mithril.git
cd mithril
go install .
```

## Uninstall

Remove the workspace and all Docker resources:

```bash
mithril clean --all
```

Then delete the binary:

```bash
rm $(which mithril)
```
