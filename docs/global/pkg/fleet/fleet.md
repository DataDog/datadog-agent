# pkg/fleet — Fleet Automation

## Purpose

`pkg/fleet` is the top-level package for Datadog Fleet Automation. It provides remote management of Datadog software on a fleet of hosts: installing packages, upgrading them, rolling back experiments, and applying remote configuration — all triggered from the Datadog backend through Remote Configuration (RC).

The package exposes a long-running **daemon** (`pkg/fleet/daemon`) that the Datadog Agent hosts inside its own process (via the `comp/updater` component). The daemon listens to RC, orchestrates all package lifecycle operations on the local host, and reports state back to the backend.

## Key elements

### pkg/fleet/daemon

#### `Daemon` interface (`daemon/daemon.go`)

The central interface that the `comp/updater` component holds. All remote-triggered operations flow through it.

```go
type Daemon interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Manual local operations (also reachable via the local HTTP API)
    Install(ctx context.Context, url string, args []string) error
    Remove(ctx context.Context, pkg string) error

    // Package experiments: install a candidate version alongside the stable one,
    // then either promote (make it stable) or stop (roll back to stable).
    StartExperiment(ctx context.Context, url string) error
    StopExperiment(ctx context.Context, pkg string) error
    PromoteExperiment(ctx context.Context, pkg string) error

    // Config experiments: apply a configuration change as an experiment,
    // then promote or stop it.
    StartConfigExperiment(ctx context.Context, pkg string, operations config.Operations, encryptedSecrets map[string]string) error
    StopConfigExperiment(ctx context.Context, pkg string) error
    PromoteConfigExperiment(ctx context.Context, pkg string) error

    // State queries
    GetPackage(pkg string, version string) (Package, error)
    GetState(ctx context.Context) (map[string]PackageState, error)
    GetRemoteConfigState() *pbgo.ClientUpdater
    GetAPMInjectionStatus() (APMInjectionStatus, error)

    // Catalog management (called from RC update handlers)
    SetCatalog(c catalog)
    SetConfigCatalog(configs map[string]installerConfig)
}
```

#### `NewDaemon` (`daemon/daemon.go`)

Constructor. Reads configuration from the agent config reader, resolves the installer binary path, creates the task database, and sets up the RC client. Called by `comp/updater/updater/updaterimpl`.

#### `PackageState` (`daemon/daemon.go`)

Holds the version and config state for a single installed package, each as a `repository.State` with `Stable` and `Experiment` string fields.

#### `Package` / `catalog` (`daemon/remote_config.go`)

`Package` is the downloadable unit as seen by the daemon. A `catalog` is a list of `Package` entries received from RC (`ProductUpdaterCatalogDD`). Each package carries Name, Version, URL (OCI digest reference), SHA256, platform, and arch.

#### Remote API methods (`daemon/remote_config.go`)

String constants for the operations RC can trigger:
- `install_package`, `uninstall_package`
- `start_experiment`, `stop_experiment`, `promote_experiment`
- `start_experiment_config`, `stop_experiment_config`, `promote_experiment_config`

#### `LocalAPI` / `localAPIImpl` (`daemon/local_api.go`)

An HTTP server bound to a local Unix socket. It exposes the same Daemon operations to local callers (e.g., the Agent CLI, `datadog-installer` commands). Starts and stops with the daemon lifecycle.

`APMInjectionStatus` is returned by the `/apm-injection-status` endpoint and exposes whether the host and Docker daemon are currently instrumented via `ld.so.preload` and `daemon.json`.

#### `remoteConfig` (`daemon/remote_config.go`)

Wraps the `pkg/config/remote/client` RC client. Subscribes to three RC products:
- `ProductInstallerConfig` — fleet-managed config patches
- `ProductUpdaterCatalogDD` — package catalog
- `ProductUpdaterTask` — install/experiment tasks (subscribed only after the first catalog is received)

#### `taskDB` (`daemon/task_db.go`)

A simple SQLite-backed persistent store (`/opt/datadog-packages/run/installer_tasks.db`) that records the state of in-progress and completed RC tasks so task status survives daemon restarts.

### Configuration keys

| Key | Default | Description |
|-----|---------|-------------|
| `remote_updates` | `false` | Must be `true` for the daemon to start and process RC tasks |
| `installer.mirror` | — | OCI mirror URL for all package downloads |
| `installer.registry.url` | — | Override the default OCI registry |
| `installer.registry.auth` | `docker` | Auth method: `docker`, `gcr`, or `password` |
| `installer.refresh_interval` | — | How often the daemon polls for state updates |
| `installer.gc_interval` | — | How often garbage collection runs |

### Secrets encryption

Config experiments may include secrets. The daemon generates a NaCl box key pair at startup and advertises the public key to RC via `ClientUpdater.SecretsPubKey`. RC encrypts secrets with this key; the daemon decrypts them in memory and passes the plaintext to the installer binary without ever writing them to disk or argv.

## Usage in the codebase

The daemon is wired into the agent via the `comp/updater` fx component tree:

```
comp/updater/updater/updaterimpl  →  daemon.NewDaemon  →  daemon.Daemon
comp/updater/localapi/impl        →  daemon.LocalAPI
comp/updater/localapiclient       →  HTTP client wrapping LocalAPI
```

`comp/updater/updater/updaterimpl/updater.go` calls `daemon.NewDaemon`, registers `Start`/`Stop` as fx lifecycle hooks, and wraps the result as the `updater.Component`. The agent only starts the daemon goroutines when `remote_updates: true` is set in `datadog.yaml`.

The local API (`LocalAPI`) is used by the `datadog-installer` CLI (via `comp/updater/localapiclient`) for imperative operations such as manual package installs and local experiment management, independently of RC.

## Cross-references

| Topic | See also |
|-------|----------|
| Package install/upgrade logic, OCI downloader, hooks | [`pkg/fleet/installer`](installer.md) |
| fx component wrapping the daemon, fx wiring, lifecycle hooks | [`comp/updater/updater`](../../comp/updater/updater.md) |
| RC client (`remoteConfig`) used by the daemon to receive tasks | [`pkg/config/remote`](../config/remote.md) |
| TUF state machine and `state.Repository` underlying RC | [`pkg/remoteconfig`](../remoteconfig.md) |
| RC service that the daemon connects to | [`comp/remote-config/rcservice`](../../comp/remote-config/rcservice.md) |

### RC product flow for Fleet

```
Datadog backend
    │  HTTPS
    ▼
comp/remote-config/rcservice  (CoreAgentService — polls backend, drives Uptane)
    │  gRPC IPC
    ▼
pkg/fleet/daemon.remoteConfig  (wraps pkg/config/remote/client.Client)
    │  subscribes to:
    │    ProductInstallerConfig   → SetConfigCatalog
    │    ProductUpdaterCatalogDD  → SetCatalog
    │    ProductUpdaterTask       → Install / StartExperiment / …
    ▼
pkg/fleet/daemon.Daemon  (orchestrates)
    │  exec subprocess
    ▼
pkg/fleet/installer.Installer  (OCI download, repository symlinks, hooks)
```
