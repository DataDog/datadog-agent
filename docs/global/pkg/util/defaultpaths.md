# pkg/util/defaultpaths

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/defaultpaths`

## Purpose

`pkg/util/defaultpaths` is the single authoritative source of **default file and directory paths** used by the agent across all supported platforms. Rather than scattering platform-specific path strings throughout the codebase, every component that needs to know where `datadog.yaml`, log files, Python checks, or the `dist` directory live imports this package.

The package uses Go's build-tag mechanism (`//go:build`) to select the correct set of paths at compile time:

| Build constraint | File | Platform |
|---|---|---|
| `linux`, `netbsd`, `openbsd`, `solaris`, `dragonfly` | `path_nix.go` | Linux and other Unix-like |
| _(no constraint, darwin only)_ | `path_darwin.go` | macOS |
| `freebsd` | `path_freebsd.go` | FreeBSD |
| `windows` | `path_windows.go` | Windows |

On Windows, several paths are runtime-determined rather than compile-time constants: `ConfPath` and `LogFile` are adjusted in `init()` from the Windows `%ProgramData%` directory (via `winutil.GetProgramDataDir()`), and the installation path is read from the Windows Registry key `SOFTWARE\DataDog\Datadog Agent\InstallPath` (falling back to `C:\Program Files\Datadog\Datadog Agent`).

## Key Elements

### Constants (all platforms except Windows)

| Constant | Typical value (Linux) | Description |
|---|---|---|
| `ConfPath` | `/etc/datadog-agent` | Directory containing `datadog.yaml` |
| `LogFile` | `/var/log/datadog/agent.log` | Default agent log file |
| `DCALogFile` | `/var/log/datadog/cluster-agent.log` | Cluster-agent log file |
| `JmxLogFile` | `/var/log/datadog/jmxfetch.log` | JMX fetch log file |
| `CheckFlareDirectory` | `/var/log/datadog/checks/` | Flare-friendly check log directory |
| `JMXFlareDirectory` | `/var/log/datadog/jmxinfo/` | Flare-friendly JMX info directory |
| `DogstatsDLogFile` | `/var/log/datadog/dogstatsd_info/dogstatsd-stats.log` | DogStatsD stats log |
| `StreamlogsLogFile` | `/var/log/datadog/streamlogs_info/streamlogs.log` | Stream-logs log |

On Windows the same names are `var` declarations (not constants) so they can be updated in `init()`.

### Variables

| Variable | Description |
|---|---|
| `PyChecksPath` | Path to the bundled Python checks directory (`checks.d`). Computed relative to the executable at package init time (or relative to the MSI install path on Windows). |

### Functions

| Function | Description |
|---|---|
| `GetDistPath() string` | Returns the path to the agent's `dist` directory (contains sub-configs, Python runtime, etc.). On Linux/macOS this is computed relative to the executable; on Windows it is relative to the install path. |
| `GetInstallPath() string` | Returns the directory containing the agent executable. On Windows this comes from the registry; on other platforms it uses `executable.Folder()`. |
| `GetDefaultConfPath() string` | Returns the directory where the agent looks for `datadog.yaml`. Equivalent to reading `ConfPath`, but provided as a function for consistency across platforms. |
| `GetEmbeddedBinPath() string` | Returns the path to the embedded binaries (e.g., Python interpreter). For the Cluster Agent flavor this is the same as `GetInstallPath()`; for other flavors it points to `embedded/bin` relative to the install root. On Windows it is always `<InstallPath>/bin`. |

## Relationship to other packages

| Package | Role |
|---|---|
| [`pkg/util/installinfo`](installinfo.md) | Reads the `install_info` YAML file whose path is derived from `ConfPath` (it lives next to `datadog.yaml`). `installinfo.GetFilePath` calls `conf.GetString("confd_path")` rather than `defaultpaths.ConfPath` directly, but both ultimately resolve to the same directory. |
| [`pkg/config/setup`](../../config/setup.md) | The primary consumer of this package. `InitConfigObjects` and the per-platform `config_nix.go` / `config_windows.go` / `config_darwin.go` files in `pkg/config/setup` export additional path constants (`DefaultSystemProbeAddress`, `DefaultProcessAgentLogFile`, etc.) that complement the paths defined here. Both packages must agree on the install layout; do not add overlapping path constants to `pkg/config/setup` if they already exist in `pkg/util/defaultpaths`. |

## Usage

### Typical usage in a command entry point

```go
import "github.com/DataDog/datadog-agent/pkg/util/defaultpaths"

// Pass the default log file path to the logger initialisation:
log.ForDaemon("Agent", "log_file", defaultpaths.LogFile)

// Pass default paths to the config loader:
config.SetupConfig(
    defaultpaths.GetDistPath(),
    defaultpaths.PyChecksPath,
    defaultpaths.LogFile,
    defaultpaths.JmxLogFile,
    defaultpaths.DogstatsDLogFile,
    defaultpaths.StreamlogsLogFile,
)
```

This pattern is used verbatim in `cmd/agent/subcommands/run/command.go`.

### Known callers

The package is imported by virtually every agent binary entry point and by many configuration-loading helpers:
- `cmd/agent/subcommands/run/command.go` (and `command_windows.go`)
- `cmd/cluster-agent/subcommands/start/command.go`
- `cmd/cluster-agent-cloudfoundry/subcommands/run/command.go`
- `comp/core/config/setup.go`
- `comp/core/gui/guiimpl/`
- `comp/core/secrets/impl/`
- `pkg/cli/subcommands/check/command.go`
- `pkg/jmxfetch/jmxfetch.go`

### Platform differences to be aware of

- **Windows:** `ConfPath` defaults to `c:\programdata\datadog` but is overridden at runtime from `%ProgramData%`. The install path is read from the Windows Registry; if the key is absent (e.g. standalone binary not installed via MSI) it falls back to `C:\Program Files\Datadog\Datadog Agent`.
- **macOS:** Paths are rooted at `/opt/datadog-agent` (brew-style install), not `/etc` or `/var`.
- **Linux:** Paths follow the FHS (`/etc/datadog-agent`, `/var/log/datadog`).
- Do **not** hard-code these paths elsewhere in the codebase; always import this package. This is especially important for packaging changes that may alter the install layout.
