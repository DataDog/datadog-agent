# pkg/fleet/installer — Package Installer

## Purpose

`pkg/fleet/installer` is the package manager at the heart of Fleet Automation. It is responsible for the full lifecycle of Datadog software packages on a single host: downloading OCI images, extracting them to a versioned on-disk repository, running package-specific hooks (service start/stop, APM injection setup, etc.), and managing stable/experiment version slots. It is also the entry point for the `datadog-installer` CLI binary.

The installer runs as a **subprocess** of the fleet daemon: the daemon calls the installer binary via `exec` rather than linking to it directly. This provides process isolation and allows the installer to self-update (the daemon can swap the installer binary between calls).

## Key elements

### `Installer` interface (`installer.go`)

The main public interface. All operations are safe to call concurrently (protected internally by a mutex).

```go
type Installer interface {
    // State queries
    IsInstalled(ctx context.Context, pkg string) (bool, error)
    AvailableDiskSpace() (uint64, error)
    State(ctx context.Context, pkg string) (repository.State, error)
    ConfigState(ctx context.Context, pkg string) (repository.State, error)
    ConfigAndPackageStates(ctx context.Context) (*repository.PackageStates, error)

    // Package lifecycle
    Install(ctx context.Context, url string, args []string) error
    ForceInstall(ctx context.Context, url string, args []string) error
    SetupInstaller(ctx context.Context, path string) error
    Remove(ctx context.Context, pkg string) error
    Purge(ctx context.Context)

    // Package experiments
    InstallExperiment(ctx context.Context, url string) error
    RemoveExperiment(ctx context.Context, pkg string) error
    PromoteExperiment(ctx context.Context, pkg string) error

    // Config experiments
    InstallConfigExperiment(ctx context.Context, pkg string, operations config.Operations, decryptedSecrets map[string]string) error
    RemoveConfigExperiment(ctx context.Context, pkg string) error
    PromoteConfigExperiment(ctx context.Context, pkg string) error

    // Extensions (integrations / plugins installed on top of a package)
    InstallExtensions(ctx context.Context, url string, extensionList []string) error
    RemoveExtensions(ctx context.Context, pkg string, extensionList []string) error
    SaveExtensions(ctx context.Context, pkg string, path string) error
    RestoreExtensions(ctx context.Context, url string, path string) error

    // Housekeeping
    GarbageCollect(ctx context.Context) error
    InstrumentAPMInjector(ctx context.Context, method string) error
    UninstrumentAPMInjector(ctx context.Context, method string) error
    Close() error
}
```

`NewInstaller(ctx, env)` creates the concrete implementation. It ensures the on-disk directories exist, opens the SQLite package database, and initialises the OCI downloader.

### Known packages (`default_packages.go`)

`PackagesList` enumerates every package the installer knows about:

| Package | Notes |
|---------|-------|
| `datadog-agent` | Released only when `remote_updates: true` |
| `datadog-apm-inject` | Released when APM SSI is enabled |
| `datadog-apm-library-{java,ruby,js,dotnet,python,php}` | Language-specific tracers |
| `datadog-ddot` | OTel Collector, released when `DD_OTELCOLLECTOR_ENABLED=true` |

`DefaultPackages(env)` resolves the list of OCI URLs to install based on the current environment (site, APM settings, version overrides).

### Sub-packages

#### `oci/` — OCI image downloader

`Downloader` fetches OCI images from remote registries or local `file://` paths.

- `Download(ctx, url)` returns a `*DownloadedPackage` (metadata only; layers are lazy).
- `DownloadedPackage.ExtractLayers(mediaType, dir)` extracts the matching layer to disk.
- `PackageURL(env, pkg, version)` builds the canonical OCI URL for a package.

**Media types** (layer identifiers in image manifests):

| Constant | Value | Layer content |
|----------|-------|---------------|
| `DatadogPackageLayerMediaType` | `application/vnd.datadog.package.layer.v1.tar+zstd` | Package binaries/files |
| `DatadogPackageConfigLayerMediaType` | `application/vnd.datadog.package.config.layer.v1.tar+zstd` | Default config files |
| `DatadogPackageInstallerLayerMediaType` | `application/vnd.datadog.package.installer.layer.v1` | Self-contained installer binary |
| `DatadogPackageExtensionLayerMediaType` | `application/vnd.datadog.package.extension.layer.v1.tar+zstd` | Extension files |

**Registry auth methods**: `RegistryAuthDefault` (`"docker"`), `RegistryAuthGCR` (`"gcr"`), `RegistryAuthPassword` (`"password"`).

Default registries tried in order (prod): `install.datadoghq.com`, `gcr.io/datadoghq`. Staging site (`datad0g.com`) uses `install.datad0g.com`. Failed downloads are retried up to 3 times for transient network errors.

#### `repository/` — versioned on-disk storage

A `Repository` manages one package's on-disk directory under `PackagesPath` (`/opt/datadog-packages/<pkg>/`). The layout is:

```
/opt/datadog-packages/datadog-agent/
├── 7.50.0/          ← extracted package files
├── 7.51.0/
├── stable -> 7.50.0  (symlink)
└── experiment -> 7.51.0  (symlink)
```

`State` holds `Stable` and `Experiment` version strings (empty when not set).

Key operations:
- `Create(ctx, name, stableSourcePath)` — initial install or reinstall.
- `SetExperiment(ctx, name, sourcePath)` — place a candidate version as experiment.
- `PromoteExperiment(ctx)` — atomically point `stable` at the experiment.
- `DeleteExperiment(ctx)` — revert `experiment` symlink back to `stable`.
- `Cleanup(ctx)` — remove version directories that are neither `stable` nor `experiment`, respecting `PreRemoveHook` callbacks.

All operations are designed to be atomic and leave the repository in a consistent state even if they fail mid-way.

`Repositories` (from `repositories.go`) is a container for all per-package `Repository` objects, keyed by package name.

#### `packages/` — package-specific hooks

`Hooks` is the interface through which the installer calls into package-specific setup and teardown logic. The implementation (`hooksCLI`) re-executes the installer binary with a serialised `HookContext` to run the hook in a fresh process, picking up the correct version of the installer binary from the package being operated on.

**Hook lifecycle** (called in this order for a full install):

```
PreInstall → (extract files) → PostInstall
```

**Experiment lifecycle**:
```
PreStartExperiment → (extract files) → PostStartExperiment
... (experiment is running) ...
PreStopExperiment  → (remove experiment dir) → PostStopExperiment   ← rollback
  OR
PrePromoteExperiment → (swap symlinks) → PostPromoteExperiment       ← promote
```

Config experiments follow the same three-step pattern via `PostStartConfigExperiment`, `PreStopConfigExperiment`, `PostPromoteConfigExperiment`.

`PackageType` constants: `PackageTypeOCI`, `PackageTypeDEB`, `PackageTypeRPM`, `PackageTypeMSI`.

Per-package hook implementations live in:
- `packages/datadog_agent_linux.go`, `packages/datadog_agent_windows.go`, `packages/datadog_agent_darwin.go` — agent start/stop/restart
- `packages/apm_inject_linux.go`, `packages/apminject/` — APM injector setup (`ld.so.preload`, Docker runtime)
- `packages/datadog_agent_extensions.go` — agent extensions handling

#### `setup/` — first-run install scenarios

`Setup(ctx, env, flavor)` orchestrates opinionated full-stack installs. Available flavors:

| Flavor | Entry point | Use case |
|--------|-------------|----------|
| `"default"` | `defaultscript.SetupDefaultScript` | Standard one-line install script |
| `"APM SSI"` | `defaultscript.SetupAPMSSIScript` | Standalone APM SSI |
| `"databricks"` | `djm.SetupDatabricks` | Data Jobs Monitoring on Databricks |
| `"emr"` | `djm.SetupEmr` | Data Jobs Monitoring on EMR |
| `"dataproc"` | `djm.SetupDataproc` | Data Jobs Monitoring on Dataproc |

`Agent7InstallScript(ctx, env)` is the thin wrapper called by the agent 7 install script: it asks the stable installer for its list of default packages and installs each one.

#### `env/` — environment configuration

`Env` holds all runtime configuration for the installer, populated from environment variables and the agent config:

| Key field | Env var | Description |
|-----------|---------|-------------|
| `APIKey` | `DD_API_KEY` | Datadog API key |
| `Site` | `DD_SITE` | Datadog site (e.g. `datadoghq.com`) |
| `RemoteUpdates` | `DD_REMOTE_UPDATES` | Enable RC-driven updates |
| `Mirror` | `DD_INSTALLER_MIRROR` | OCI mirror URL |
| `RegistryOverride` | `DD_INSTALLER_REGISTRY_URL` | Override OCI registry |
| `RegistryAuthOverride` | `DD_INSTALLER_REGISTRY_AUTH` | Override auth method |
| `InstallScript.*` | `DD_APM_INSTRUMENTATION_ENABLED`, etc. | First-run install options |

Per-image registry overrides are read from `DD_INSTALLER_REGISTRY_URL_<IMAGE>` environment variables.

`FromEnv()` creates an `Env` from the process environment. `HTTPClient()` returns an `*http.Client` that respects the proxy settings in the env.

#### `paths/` — well-known filesystem paths

| Constant | Linux value | Description |
|----------|-------------|-------------|
| `PackagesPath` | `/opt/datadog-packages` | Root for all installed packages |
| `ConfigsPath` | `/etc/datadog-agent/managed` | Fleet-managed config directory |
| `AgentConfigDir` | `/etc/datadog-agent` | Stable agent config |
| `AgentConfigDirExp` | `/etc/datadog-agent-exp` | Experiment agent config |
| `StableInstallerPath` | `/opt/datadog-packages/datadog-installer/stable/bin/installer/installer` | Stable installer binary |
| `RunPath` | `/opt/datadog-packages/run` | PID files, task DB, sockets |

Windows uses different values defined in `installer_paths_windows.go`.

#### `db/` — package database

A SQLite database (`packages.db`) inside `PackagesPath` records which packages are installed and at which version. It is the authoritative source of truth for `IsInstalled` checks and is consulted during upgrades and purges.

#### `config/` — config experiment engine

Applies JSON Merge Patch operations (`Operations`) to agent configuration files. Supports secret placeholder substitution (`SEC[key]`) after the daemon decrypts the values. Manages stable/experiment config directory pairs.

#### `exec/` — subprocess wrapper

`InstallerExec` runs the installer binary as a subprocess, forwarding telemetry trace context via environment variables. Used by both `hooksCLI` (to re-exec hooks) and the daemon (to call installer operations safely across self-update boundaries).

`GetExecutable()` returns the path of the currently running installer binary.

## Usage in the codebase

The installer is used in two modes:

**1. As a library** — `pkg/fleet/daemon` imports `installer.Installer` (constructed via `exec.NewInstallerExec`, which wraps the subprocess). The daemon never calls `NewInstaller` directly in production; it always shells out so that the correct version of the installer binary is used.

**2. As a CLI binary** — `cmd/installer/` (the `datadog-installer` binary) uses `pkg/fleet/installer/commands` to build the Cobra command tree, calling `NewInstaller` directly for the current process. This binary is what gets installed into `/opt/datadog-packages/datadog-installer/stable/`.

**Setup / first-run** — `pkg/fleet/installer/setup` is called from the install script and from the agent 7 install script. `setup.Agent7InstallScript` is the entry point used when the user runs `DD_FLEET_AUTOMATION=true` with the standard install script.

**Extensions** — `pkg/fleet/installer/packages/extensions` manages the install/remove/save/restore of Agent integrations (extensions) layered on top of the base agent package. After any extension change the agent is restarted via `packages.RestartDatadogAgent`.

### Install info reporting

After a successful installation the installer calls `pkg/util/installinfo.WriteInstallInfo` to record that packages were installed by the Fleet Installer. This value propagates to inventory payloads sent to the Datadog backend. See [`pkg/util/installinfo`](../util/installinfo.md) for the full priority chain and the HTTP handler used to update the value at runtime.

## Cross-references

| Topic | See also |
|-------|----------|
| Daemon that drives the installer via subprocess exec | [`pkg/fleet`](fleet.md) |
| fx component wrapping the fleet daemon | [`comp/updater/updater`](../../comp/updater/updater.md) |
| Install info metadata written after installation | [`pkg/util/installinfo`](../util/installinfo.md) |
| ZIP / tar.xz archive utilities used by OCI layer extraction | [`pkg/util/archive`](../util/archive.md) |

### How the installer fits into the full Fleet stack

The installer is the lowest layer in a three-tier stack. The `pkg/fleet/daemon` layer orchestrates tasks received from Remote Configuration (see [`pkg/fleet`](fleet.md)) and shells out to the installer binary for every package operation. The `comp/updater/updater` fx component (see [`comp/updater/updater`](../../comp/updater/updater.md)) wraps the daemon as a lifecycle-managed component inside the agent process, wiring it to the RC service and the local Unix-socket API.

```
comp/updater/updater/updaterimpl   ← fx lifecycle wrapper
    │  daemon.NewDaemon(hostname, rcservice, config)
    ▼
pkg/fleet/daemon.Daemon            ← RC polling, task DB, secrets decryption
    │  exec.InstallerExec.Install / StartExperiment / …
    ▼
pkg/fleet/installer.Installer      ← OCI download, repository symlinks, hooks
    │  pkg/util/installinfo.WriteInstallInfo
    ▼
/opt/datadog-packages/<pkg>/       ← versioned on-disk layout
```

### OCI layer extraction and `pkg/util/archive`

The `oci/` sub-package extracts layers from downloaded OCI images directly via `archive/tar` and `compress/zstd`. The higher-level ZIP and tar.xz utilities in [`pkg/util/archive`](../util/archive.md) (used for flare bundles and eBPF BTF archives) are not used by the installer — each is an independent extraction code path. Both share the `github.com/cyphar/filepath-securejoin` strategy for path traversal protection.

### Install info written after installation

After a successful `Install` or `ForceInstall` call, the installer writes install metadata via `pkg/util/installinfo.WriteInstallInfo`. The `Tool` value is set to `"fleet_installer"` so inventory payloads (sent by `comp/metadata/inventoryagent`) correctly report the installation method. The runtime HTTP handler `HandleSetInstallInfo` (registered at `comp/api/api/apiimpl`) lets the installer update the value without restarting the agent. See [`pkg/util/installinfo`](../util/installinfo.md) for the full priority chain.

### Subprocess exec model

The installer binary is self-contained: the `exec/` sub-package launches a new process and forwards the telemetry trace context through environment variables. This design lets the daemon upgrade the installer binary and immediately call the new version without a daemon restart:

```
pkg/fleet/daemon
    │  InstallerExec.Install(ctx, url, args)
    │  (sets DD_INSTALLER_TRACE_ID env var)
    ▼
/opt/datadog-packages/datadog-installer/stable/bin/installer/installer
    │  NewInstaller(ctx, env)
    │  → oci.Downloader.Download(url)
    │  → repository.Repository.Create(...)
    │  → packages.Hooks.PostInstall(...)
    ▼
/opt/datadog-packages/<pkg>/<version>/   (extracted files)
/opt/datadog-packages/<pkg>/stable       (symlink updated atomically)
```
