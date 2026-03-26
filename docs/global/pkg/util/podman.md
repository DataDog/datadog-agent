# pkg/util/podman

## Purpose

`pkg/util/podman` is a lightweight, read-only client for Podman's state databases. Instead of importing the full Podman Go library (which brings heavyweight and sometimes incompatible dependencies), this package reads Podman's BoltDB or SQLite state databases directly to retrieve container metadata.

The only operation needed by the agent is listing all existing containers. The implementation is deliberately minimal: it opens the database in read-only mode, reads container config and state JSON blobs, and deserializes them into local struct types copied from Podman's source.

The package is only compiled when the `podman` build tag is set.

## Key elements

### Build tag

```
//go:build podman
```

All non-doc files require this tag.

### `DBClient` (BoltDB, Podman ≤ v4)

```go
type DBClient struct { DBPath string }
func NewDBClient(dbPath string) *DBClient
func (c *DBClient) GetAllContainers() ([]Container, error)
```

Opens the BoltDB at `DBPath` in read-only mode with a 30-second lock timeout, iterates the `all-ctrs` bucket, and for each container ID reads the `config` and `state` keys from the `ctr` bucket.

Default database path used by the workloadmeta collector: `/var/lib/containers/storage/libpod/bolt_state.db`
Rootless path suffix: `~/.local/share/containers/storage/libpod/bolt_state.db`

### `SQLDBClient` (SQLite, Podman ≥ v4.9 / v5)

```go
type SQLDBClient struct { DBPath string }
func NewSQLDBClient(dbPath string) *SQLDBClient
func (c *SQLDBClient) GetAllContainers() ([]Container, error)
```

Opens the SQLite file in query-only mode with a busy timeout of 100 seconds, then executes:

```sql
SELECT ContainerConfig.JSON, ContainerState.JSON AS StateJSON
FROM ContainerConfig
INNER JOIN ContainerState ON ContainerConfig.ID = ContainerState.ID;
```

Default database path: `/var/lib/containers/storage/db.sql`
Rootless path suffix: `~/.local/share/containers/storage/db.sql`

### `Container`

```go
type Container struct {
    Config *ContainerConfig
    State  *ContainerState
}
```

The types are copied from Podman v3.4.1 (`libpod/`) with minor modifications to remove dependencies that could not be brought in (e.g., `IDMappings`, `HealthCheckConfig`, `NetMode`).

### `ContainerState`

Holds the current runtime state:
- `State ContainerStatus` — one of `Unknown`, `Configured`, `Created`, `Running`, `Stopped`, `Paused`, `Exited`, `Removing`, `Stopping`
- `StartedTime`, `FinishedTime time.Time`
- `PID int`
- `NetworkStatus []*cnitypes.Result`
- `RestartCount uint`

### `ContainerConfig`

Holds the immutable creation config:
- `ID`, `Name`, `Pod`, `Namespace` — identity fields
- `Spec *spec.Spec` — OCI runtime spec
- `RootfsImageID`, `RootfsImageName` — image identity
- Embedded sub-configs: `ContainerSecurityConfig`, `ContainerNameSpaceConfig`, `ContainerNetworkConfig`, `ContainerImageConfig`, `ContainerMiscConfig`

### Compatibility note

The BoltDB client was written against Podman v3.4.1. The SQLite client was written against Podman v4.9.2 / v5.0.0. Future schema changes in Podman may break these clients.

## Usage

The package is consumed exclusively by the **Podman workloadmeta collector** at `comp/core/workloadmeta/collectors/internal/podman/podman.go`.

The collector creates one or more `DBClient` / `SQLDBClient` instances (one per detected database file — covering both root and rootless Podman installations), calls `GetAllContainers()` on each polling cycle, and translates the resulting `Container` values into workloadmeta container entities.

```go
client := podman.NewDBClient(dbPath)           // BoltDB
// or
client := podman.NewSQLDBClient(dbPath)        // SQLite

containers, err := client.GetAllContainers()
for _, ctr := range containers {
    // ctr.Config.ID, ctr.Config.Name, ctr.State.State, ...
}
```

The workloadmeta collector also detects which database format is present (BoltDB vs. SQLite) at startup by checking whether the respective file paths exist.
