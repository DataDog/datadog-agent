> **TL;DR:** `comp/updater/updater` wraps the Fleet daemon as an fx-managed component, handling remote-triggered install, update, and rollback of Datadog packages on the host via a local Unix socket or Windows named pipe API.

# comp/updater/updater — Updater Daemon Component

**Import path (interface):** `github.com/DataDog/datadog-agent/comp/updater/updater`
**Import path (implementation):** `github.com/DataDog/datadog-agent/comp/updater/updater/updaterimpl`
**Team:** fleet / windows-products
**Importers:** `comp/updater/localapi`

## Purpose

`comp/updater/updater` wraps the Fleet daemon (`pkg/fleet/daemon`) as an fx-managed component. The daemon is responsible for the lifecycle of Datadog packages (agent, APM injector, etc.) on the host: it listens to Remote Configuration for install, update, and rollback commands; manages stable and experiment package versions; and exposes a local Unix/named-pipe API consumed by the installer CLI.

The component exists so the long-running daemon can participate in the standard fx `Lifecycle` (Start/Stop hooks) alongside the rest of the agent.

## Package layout

| Path | Role |
|---|---|
| `comp/updater/updater/` | `Component` interface definition (`component.go`) |
| `comp/updater/updater/updaterimpl/` | `Module()` function and `newUpdaterComponent` constructor |
| `comp/updater/localapi/` | Local HTTP/Unix-socket API served on top of the Component |
| `pkg/fleet/daemon/` | Core daemon logic (package install, RC polling, task DB) |

## Key elements

### Key interfaces

```go
// Component embeds the fleet daemon interface directly.
type Component interface {
    daemon.Daemon
}
```

The `daemon.Daemon` interface (defined in `pkg/fleet/daemon`) is:

```go
type Daemon interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    SetCatalog(c catalog)
    SetConfigCatalog(configs map[string]installerConfig)
    Install(ctx context.Context, url string, args []string) error
    Remove(ctx context.Context, pkg string) error
    StartExperiment(ctx context.Context, url string) error
    StopExperiment(ctx context.Context, pkg string) error
    PromoteExperiment(ctx context.Context, pkg string) error
    StartConfigExperiment(ctx context.Context, pkg string, operations config.Operations, encryptedSecrets map[string]string) error
    StopConfigExperiment(ctx context.Context, pkg string) error
    PromoteConfigExperiment(ctx context.Context, pkg string) error

    GetPackage(pkg string, version string) (Package, error)
    GetState(ctx context.Context) (map[string]PackageState, error)
    GetRemoteConfigState() *pbgo.ClientUpdater
    GetAPMInjectionStatus() (APMInjectionStatus, error)
}
```

### Key functions

`updaterimpl.Module()` provides `updatercomp.Component` via `newUpdaterComponent`.

The constructor calls `daemon.NewDaemon(hostname, remoteConfig, config)` which:

1. Resolves the installer binary path on disk.
2. Opens a SQLite task database (`installer_tasks.db` in `paths.RunPath`).
3. Builds a Remote Config client from `rcservice` (a `pkg/config/remote/client.Client` that subscribes to `ProductInstallerConfig`, `ProductUpdaterCatalogDD`, and `ProductUpdaterTask`).
4. Generates an ephemeral NaCl keypair for encrypted secret delivery.

`Start`/`Stop` are registered as fx lifecycle hooks so the daemon starts and stops with the application.

Note: unlike most components that use `comp/remote-config/rcclient`, the fleet daemon constructs its own RC client directly from `rcservice.Component` rather than going through the `rcclient` fx component. This allows the daemon to manage its own subscription lifecycle and task database independently.

### Configuration and build flags

**fx wiring dependencies:**

| Dependency | Type | Notes |
|---|---|---|
| `hostname` | `hostname.Component` | Used to identify this host in RC messages |
| `log` | `log.Component` | Logging |
| `config` | `config.Component` | Agent configuration (API key, site, proxy, …) |
| `RemoteConfig` | `option.Option[rcservice.Component]` | **Required** — returns `errRemoteConfigRequired` if absent |

**Runtime configuration options:**

| Key | Description |
|---|---|
| `remote_updates` | Enables remote-triggered package updates |
| `installer.mirror` | OCI registry mirror for package downloads |
| `installer.registry.url` | Override the default OCI registry URL |
| `installer.refresh_interval` | How often the daemon polls for RC updates |
| `installer.gc_interval` | How often old package versions are garbage-collected |

## Usage in the codebase

The only direct consumer of `updatercomp.Component` today is `comp/updater/localapi`:

```go
// comp/updater/localapi/impl/localapi.go
func NewComponent(reqs Requires) (Provides, error) {
    localAPI, err := daemon.NewLocalAPI(reqs.Updater)
    ...
    reqs.Lifecycle.Append(compdef.Hook{OnStart: localAPI.Start, OnStop: localAPI.Stop})
    return Provides{Comp: localAPI}, nil
}
```

`NewLocalAPI` exposes the daemon's methods over a Unix socket (Linux/macOS) or a Windows named pipe, allowing the `datadog-installer` CLI to issue install/experiment commands to the running daemon.

## Related components and packages

| Component / Package | Doc | Relationship |
|---|---|---|
| `pkg/fleet/daemon` | [../../pkg/fleet/fleet.md](../../pkg/fleet/fleet.md) | Core daemon logic that this component wraps. `NewDaemon` is called by `newUpdaterComponent` and its `Start`/`Stop` are registered as fx lifecycle hooks. The daemon owns the RC subscription, SQLite task DB, secrets decryption, and subprocess exec to the installer. |
| `pkg/fleet/installer` | [../../pkg/fleet/installer.md](../../pkg/fleet/installer.md) | The subprocess that the daemon execs for every package operation (install, experiment, promote, rollback). The daemon never imports `installer.Installer` directly in production; it uses `exec.InstallerExec` to shell out to the stable installer binary at `/opt/datadog-packages/datadog-installer/stable/`. |
| `comp/remote-config/rcclient` | [../remote-config/rcclient.md](../remote-config/rcclient.md) | The daemon builds its own RC client from `rcservice.Component` rather than using the `rcclient` fx component. `rcservice.Component` is the shared backend-facing gateway; the daemon subscribes independently to Fleet-specific products (`ProductInstallerConfig`, `ProductUpdaterCatalogDD`, `ProductUpdaterTask`). |
| `comp/updater/localapi` | — | The only direct consumer of this component. It wraps the daemon's methods over a Unix socket (Linux/macOS) or Windows named pipe so the `datadog-installer` CLI can issue commands to the running daemon. |
