# Fleet automation and the installer

-----

Fleet Automation lets the Datadog backend install, upgrade, and configure Agents remotely. The engine is the `installer` binary ([`cmd/installer`](<<<SRC>>>/cmd/installer), core logic in [`pkg/fleet/installer`](<<<SRC>>>/pkg/fleet/installer)), which plays three roles: a package manager CLI for OCI-distributed packages, the `postinst` engine invoked by deb/rpm maintainer scripts (see [Packaging](packaging.md)), and a long-running daemon (`installer run`) that executes remote-config-driven upgrades with automatic rollback. Its central trick is an atomic `stable`/`experiment` symlink repository under `/opt/datadog-packages`, paired with `-exp` systemd unit twins whose failure path is a rollback to stable.

## Key packages and files

| Path | Purpose |
|---|---|
| [`cmd/installer/`](<<<SRC>>>/cmd/installer) | Binary entry point: package verbs, `postinst` hooks, `setup --flavor`, the daemon's `run` |
| [`pkg/fleet/installer/installer.go`](<<<SRC>>>/pkg/fleet/installer/installer.go) | Core package manager: `Install`, `InstallExperiment`, `PromoteExperiment`, extensions, APM injector verbs |
| [`pkg/fleet/installer/commands/`](<<<SRC>>>/pkg/fleet/installer/commands) | CLI command definitions (install/remove/experiment/APM verbs) |
| [`pkg/fleet/installer/oci/download.go`](<<<SRC>>>/pkg/fleet/installer/oci/download.go) | OCI image download, registry/mirror resolution, platform variant selection |
| [`pkg/fleet/installer/repository/repository.go`](<<<SRC>>>/pkg/fleet/installer/repository/repository.go) | The `stable`/`experiment` symlink repository with atomic flips and garbage collection |
| [`pkg/fleet/installer/db/db.go`](<<<SRC>>>/pkg/fleet/installer/db/db.go) | `packages.db` (BoltDB) tracking installed packages |
| [`pkg/fleet/installer/paths/`](<<<SRC>>>/pkg/fleet/installer/paths) | Platform paths: `/opt/datadog-packages` vs `C:\ProgramData\Datadog\Installer` |
| [`pkg/fleet/installer/packages/`](<<<SRC>>>/pkg/fleet/installer/packages) | Per-package install hooks (Go functions); [`README.md`](<<<SRC>>>/pkg/fleet/installer/packages/README.md) documents the hook model |
| [`pkg/fleet/installer/packages/embedded/tmpl/`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl) | systemd unit templates; rendered stable and `-exp` units under `gen/` |
| [`pkg/fleet/installer/default_packages.go`](<<<SRC>>>/pkg/fleet/installer/default_packages.go) | Catalog of default packages and the conditions that gate them |
| [`pkg/fleet/installer/setup/`](<<<SRC>>>/pkg/fleet/installer/setup) | Install-script flavors: default, APM SSI, Data Jobs Monitoring (`djm/`) |
| [`pkg/fleet/installer/setup/install.sh`](<<<SRC>>>/pkg/fleet/installer/setup/install.sh) | The shell bootstrapper behind `install.datadoghq.com` |
| [`pkg/fleet/installer/env/env.go`](<<<SRC>>>/pkg/fleet/installer/env/env.go) | All `DD_INSTALLER_*` / `DD_APM_INSTRUMENTATION_*` environment inputs |
| [`pkg/fleet/daemon/daemon.go`](<<<SRC>>>/pkg/fleet/daemon/daemon.go) | The fleet daemon: remote-config subscriptions, task execution |
| [`pkg/fleet/daemon/local_api.go`](<<<SRC>>>/pkg/fleet/daemon/local_api.go) | Local HTTP API over UDS/named pipe |
| [`comp/updater/updater/impl/updater.go`](<<<SRC>>>/comp/updater/updater/impl/updater.go) | Fx component hosting the daemon inside `installer run` |
| [`pkg/fleet/installer/packages/apm_inject_linux.go`](<<<SRC>>>/pkg/fleet/installer/packages/apm_inject_linux.go) | APM single-step instrumentation host/docker injector |

## OCI packages

Fleet packages are OCI images pulled from `oci://install.datadoghq.com/<name>-package:<version>`, where `<name>` is the package name without its `datadog-` prefix — `datadog-agent` becomes `agent-package` (`install.datad0g.com` for staging). [`oci/download.go`](<<<SRC>>>/pkg/fleet/installer/oci/download.go) resolves the URL, honoring a mirror (`DD_INSTALLER_MIRROR` / `installer.mirror`) or a private registry (`DD_INSTALLER_REGISTRY_URL`, `DD_INSTALLER_REGISTRY_AUTH`). The FIPS variant of a package is selected at download time through the OCI `Platform.Variant` field — same URL, different image variant. The same agent file tree that goes into the deb/rpm (see [Packaging](packaging.md)) is the payload; what differs is the install hooks and where the tree lands.

## The on-disk repository

Each package lives under `/opt/datadog-packages/<package>/` ([`repository.go`](<<<SRC>>>/pkg/fleet/installer/repository/repository.go)):

```text
/opt/datadog-packages/
├── packages.db                            # BoltDB inventory of installed packages
├── run/
│   ├── installer.sock                     # daemon local API (Unix)
│   └── datadog-agent/<version> -> /opt/datadog-agent   # bridge for deb/rpm installs
└── datadog-agent/
    ├── 7.68.0/                            # immutable version directories
    ├── 7.69.0/
    ├── stable     -> 7.68.0               # what runs normally
    └── experiment -> 7.69.0               # what runs during an upgrade experiment
                                           # (experiment -> stable when no experiment is active)
```

All mutations are atomic symlink flips; garbage (orphaned version directories) is collected on the next operation. Configuration has a parallel stable/experiment scheme: `/etc/datadog-agent` versus `/etc/datadog-agent-exp` (see `config.Directories` in [`installer.go`](<<<SRC>>>/pkg/fleet/installer/installer.go)), which lets fleet-managed *config* changes go through the same experiment/promote lifecycle as binaries.

deb/rpm installs are bridged into this layout: the package pre-creates `stable → run/datadog-agent/<version> → /opt/datadog-agent`, so the daemon manages package-manager installs and OCI installs uniformly. On Windows, the equivalent state lives under `C:\ProgramData\Datadog\Installer` with a locked-down security descriptor ([`installer_paths_windows.go`](<<<SRC>>>/pkg/fleet/installer/paths/installer_paths_windows.go)); the daemon refuses to start if the directory ACLs are insecure.

## Hooks

Every package name maps to a set of Go hook functions ([`packages/README.md`](<<<SRC>>>/pkg/fleet/installer/packages/README.md)). deb, rpm, and OCI installs share `PostInstall`/`PreRemove` (with an upgrade flag); OCI installs additionally get experiment hooks (`Pre`/`PostStartExperiment`, `Pre`/`PostStopExperiment`, `Pre`/`PostPromoteExperiment`) and extension hooks. For `datadog-agent` on Linux the hooks live in [`datadog_agent_linux.go`](<<<SRC>>>/pkg/fleet/installer/packages/datadog_agent_linux.go): user creation, ownership, symlinks, SELinux, and writing systemd units rendered from the embedded templates — deb/rpm units go to `/lib/systemd/system` (deb) or `/usr/lib/systemd/system` (rpm), OCI-installed units to `/etc/systemd/system`.

## The daemon and remote upgrades

`installer run` (systemd unit `datadog-agent-installer.service`, `BindsTo=datadog-agent.service`, runs as root; a Windows service via `go-svc`) builds an Fx app ([`cmd/installer/subcommands/daemon/run.go`](<<<SRC>>>/cmd/installer/subcommands/daemon/run.go)) hosting [`pkg/fleet/daemon`](<<<SRC>>>/pkg/fleet/daemon/daemon.go) through the [`comp/updater`](<<<SRC>>>/comp/updater/updater/impl/updater.go) bundle. It runs its own [remote config](../configuration/remote-config.md) service instance with a dedicated database (`remote-config-installer.db`), independent of the core agent's, and subscribes to three products:

1. `UPDATER_CATALOG_DD` — the catalog of available package versions.
1. `INSTALLER_CONFIG` — fleet-managed configuration: file operations and patches applied to `datadog.yaml`, `system-probe.yaml`, `otel-config.yaml`, and friends under the `managed/` config layer. Config secrets are sealed with NaCl box keypairs.
1. `UPDATER_TASK` — remote API requests: start/stop/promote experiment, install, remove.

The daemon never mutates packages in-process: every operation shells out to the installer binary itself (`exec.NewInstallerExec`), so a crash mid-operation cannot corrupt daemon state. It reports progress and state back through the RC client (`GetRemoteConfigState`).

If `remote_updates` is false the daemon exits immediately at startup — but note the setting defaults to **true** ([`common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go)), so a plain deb install runs the daemon.

### The experiment flow on Linux

```text
backend (Fleet UI)                 installer daemon                       systemd
      |                                  |                                   |
      |-- UPDATER_TASK: start(v2) ------>|                                   |
      |                                  |-- download OCI v2                 |
      |                                  |   /opt/datadog-packages/datadog-agent/v2
      |                                  |-- experiment symlink -> v2        |
      |                                  |-- run v2 PostStartExperiment ---->| stop  datadog-agent.service
      |                                  |                                   | start datadog-agent-exp.service
      |                                  |                                   |   ExecStart=timeout 3000s agent run
      |                                  |                                   |     -c /etc/datadog-agent-exp ...
      |<-- health signals (telemetry) ---|                                   |
      |                                  |                                   |
      |-- promote(v2) ------------------>|-- stable symlink -> v2            |
      |                                  |-- restart stable units            |  (upgrade done)
      |                                  |                                   |
      |   ...or no promote / crash:      |                                   | timeout kills -exp process
      |                                  |                                   | OnFailure=datadog-agent.service
      |                                  |                                   |  -> stable agent restarts (rollback)
```

The safety net is entirely declarative, in the [`datadog-agent-exp.service`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm/datadog-agent-exp.service) unit: `Conflicts=datadog-agent.service`, `OnFailure=datadog-agent.service`, `Restart=no`, and `ExecStart=/usr/bin/timeout --kill-after=15s $EXPERIMENT_TIMEOUT ...` with `EXPERIMENT_TIMEOUT=3000s`. If the backend does not promote within 50 minutes, or the experiment process dies, `timeout(1)` kills it, the unit fails, and `OnFailure=` restarts stable — an unattended rollback that works even if the daemon itself is the thing being upgraded. `ExecStopPost=/bin/false` forces the unit into a failed state so `OnFailure` fires even on a clean exit. Every agent unit has such an `-exp` twin; the experiment agent runs with `-c /etc/datadog-agent-exp`, keeping stable config untouched. See [Process supervision](../processes/supervision.md) for the full unit tree.

### Local API

The daemon serves a plain-HTTP JSON API for the CLI and E2E tests — no bearer token; access control is the socket itself ([`local_api_unix.go`](<<<SRC>>>/pkg/fleet/daemon/local_api_unix.go), [`local_api_windows.go`](<<<SRC>>>/pkg/fleet/daemon/local_api_windows.go)):

| Transport | Path | Access |
|---|---|---|
| Unix domain socket | `/opt/datadog-packages/run/installer.sock` | mode 0700 |
| Windows named pipe | `\\.\pipe\DD_INSTALLER` | SYSTEM and Administrators only |

Routes ([`local_api.go`](<<<SRC>>>/pkg/fleet/daemon/local_api.go)): `GET /status`, `POST /catalog`, `POST /{package}/experiment/{start,stop,promote}`, `POST /{package}/config_experiment/...`, `POST /{package}/install`, `/remove`, plus `pprof` endpoints.

## Bootstrap: install.sh and setup flavors

The one-liner install script behind `install.datadoghq.com` is generated from [`setup/install.sh`](<<<SRC>>>/pkg/fleet/installer/setup/install.sh): it downloads a static installer binary (SHA256-pinned per architecture), verifies it, and runs `installer setup --flavor <flavor>`. Flavors ([`setup/setup.go`](<<<SRC>>>/pkg/fleet/installer/setup/setup.go)):

1. `default` ([`defaultscript/`](<<<SRC>>>/pkg/fleet/installer/setup/defaultscript)) — installs the agent deb/rpm (or the OCI `datadog-agent` package when `DD_REMOTE_UPDATES=true`) plus optional APM SSI.
1. APM SSI standalone — injector and language libraries only.
1. Data Jobs Monitoring — `databricks`, `emr`, `dataproc` ([`setup/djm/`](<<<SRC>>>/pkg/fleet/installer/setup/djm)), which configure the agent for Spark clusters.

What `setup` installs is decided by [`default_packages.go`](<<<SRC>>>/pkg/fleet/installer/default_packages.go): each entry has a release gate and a condition function. `datadog-agent` and `datadog-ddot` are gated on `RemoteUpdates`; `datadog-apm-inject` and the per-language `datadog-apm-library-*` packages are driven by `DD_APM_INSTRUMENTATION_ENABLED` (`all`/`host`/`docker`) and `DD_APM_INSTRUMENTATION_LIBRARIES`. Per-package overrides exist: `DD_INSTALLER_DEFAULT_PKG_INSTALL_<PKG>` and `DD_INSTALLER_DEFAULT_PKG_VERSION_<PKG>`.

## APM single-step instrumentation (SSI)

`InstrumentAPMInjector`/`UninstrumentAPMInjector` are first-class installer verbs ([`apm_inject_linux.go`](<<<SRC>>>/pkg/fleet/installer/packages/apm_inject_linux.go)):

1. **Host method** — writes the injector library into `/etc/ld.so.preload` so every newly exec'd process loads it; a `datadog-apm-inject` systemd oneshot unit re-asserts the entry on boot (degrading gracefully when systemd is absent). The injector then loads the matching `datadog-apm-library-<lang>` tracer into supported runtimes.
1. **Docker method** — edits `/etc/docker/daemon.json` to install an OCI runtime wrapper, instrumenting containers instead of host processes.

On Windows the equivalent packages install IIS/.NET instrumentation ([`apm_library_dotnet_windows.go`](<<<SRC>>>/pkg/fleet/installer/packages/apm_library_dotnet_windows.go), `datadog-apm-library-iis`).

## Configuration

| Setting / env | Effect |
|---|---|
| `remote_updates` (`DD_REMOTE_UPDATES`) | Enables the fleet daemon and makes `datadog-agent` installable as an OCI package; default true |
| `installer.mirror` (`DD_INSTALLER_MIRROR`) | Pull OCI packages through a mirror |
| `installer.registry.url` / `.auth` / `.username` / `.password` | Private registry override |
| `installer.refresh_interval` (30s), `installer.gc_interval` (1h) | Daemon cadence knobs |
| `DD_INSTALLER_DEFAULT_PKG_INSTALL_<PKG>`, `DD_INSTALLER_DEFAULT_PKG_VERSION_<PKG>` | Force/pin default packages at setup time |
| `DD_APM_INSTRUMENTATION_ENABLED`, `DD_APM_INSTRUMENTATION_LIBRARIES` | SSI method (`all`/`host`/`docker`) and language library set |
| `DD_OTELCOLLECTOR_ENABLED` | Install `datadog-ddot` (see [DDOT](../otel/ddot.md)) |
| `DD_FLEET_POLICIES_DIR` | Points agent processes at the fleet-managed config layer (`/etc/datadog-agent/managed/datadog-agent/stable`); set by the systemd units |

## Gotchas

1. **Rollback is systemd's job, not the daemon's.** The `timeout(1)` + `OnFailure=` mechanics mean rollback works even when the daemon crashes mid-upgrade. Seeing `datadog-agent-exp.service` running under `/usr/bin/timeout` is a healthy in-progress upgrade, not a problem.
1. **Remote upgrade of a deb/rpm host forks reality.** The new version is installed as an OCI package under `/opt/datadog-packages` while dpkg/rpm still believes the old version is installed. `dpkg -l datadog-agent` is no longer the truth after the first fleet upgrade; `datadog-installer status` (or `packages.db`) is.
1. **The daemon runs by default.** `remote_updates` defaults to true, so every fresh deb install starts `datadog-agent-installer.service`. When the flag is false the daemon exits silently on Linux (unit goes inactive) and stops the service gracefully on Windows — "inactive (dead)" is not a failure.
1. **The installer executes itself.** Operations run via `exec.NewInstallerExec` against the *target version's* installer binary, so new-version hooks run new-version code. Debugging an upgrade means looking at logs of a short-lived child process, not only the daemon.
1. **install.sh handles sudo-rs.** Ubuntu 25's sudo-rs lacks `sudo -E`; the script explicitly re-passes `DD_*` and proxy variables instead of relying on environment preservation.
1. **Version handoff.** If `DD_AGENT_MAJOR_VERSION`/`DD_AGENT_MINOR_VERSION` request a specific version, `setup` downloads and re-execs the matching versioned installer, so the running installer version may not be the one you downloaded.
