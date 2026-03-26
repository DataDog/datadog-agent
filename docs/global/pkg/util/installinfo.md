# pkg/util/installinfo

## Purpose

`pkg/util/installinfo` reads and writes metadata that describes how the Datadog Agent was
installed. This metadata is included in inventory payloads sent to the Datadog backend so that
operators can track which tool deployed each agent instance and at which version.

The package understands three sources for this information, checked in priority order:

1. **Runtime override** — set programmatically via an HTTP handler (highest priority).
2. **Environment variables** — `DD_INSTALL_INFO_TOOL`, `DD_INSTALL_INFO_TOOL_VERSION`,
   `DD_INSTALL_INFO_INSTALLER_VERSION`.
3. **`install_info` file** — a YAML file written next to the agent configuration by the
   installation tool (lowest priority).

All values are scrubbed through `pkg/util/scrubber` before being returned.

## Key elements

### InstallInfo

```go
type InstallInfo struct {
    Tool             string `json:"tool"              yaml:"tool"`
    ToolVersion      string `json:"tool_version"      yaml:"tool_version"`
    InstallerVersion string `json:"installer_version" yaml:"installer_version"`
}
```

The core data structure. Examples of `Tool` values: `"chef"`, `"ansible"`, `"dpkg"`,
`"msi-installer"`, `"helm"`, `"datadog-operator"`.

### Functions

**`Get(conf model.Reader) (*InstallInfo, error)`**

Primary read entry point. Checks runtime override, then env vars, then the `install_info` file.
Returns an error only if all three sources are unavailable or malformed.

**`GetFilePath(conf model.Reader) string`**

Returns the path to the `install_info` file (sibling of the agent configuration directory).

**`WriteInstallInfo(tool, toolVersion, installType string) error`** *(Linux/macOS only)*

Writes the `install_info` YAML file and a companion `install.json` signature file
(`/etc/datadog-agent/install.json`). The signature file contains a random install UUID,
install type, and Unix timestamp. On Windows this is a no-op; the MSI installer handles it.

**`RmInstallInfo()` ** *(Linux/macOS only)*

Removes both files. Used during uninstallation.

**`LogVersionHistory()`**

Appends a new entry to `<run_path>/version-history.json` whenever the running agent version
differs from the last recorded version. Each entry captures the agent version, a UTC timestamp,
and the current `InstallInfo`. The file is capped at 60 entries (oldest trimmed first).

### HTTP handlers

**`HandleSetInstallInfo(w http.ResponseWriter, r *http.Request)`**

`POST` handler that accepts a JSON body with `tool`, `tool_version`, and `installer_version` and
sets the runtime override. All three fields are required. Registered at the agent's internal
IPC API so that the Datadog Installer can update the value after installation without restarting
the agent.

**`HandleGetInstallInfo(w http.ResponseWriter, r *http.Request)`**

`GET` handler that returns the current `InstallInfo` as JSON using the same `Get()` priority
chain.

### Environment variable override

If all three of the following variables are set, they are used as the install info source.  If
only a subset are set, the package logs a warning and ignores all of them (partial env overrides
are rejected to avoid inconsistent metadata).

| Variable | Corresponding field |
|---|---|
| `DD_INSTALL_INFO_TOOL` | `Tool` |
| `DD_INSTALL_INFO_TOOL_VERSION` | `ToolVersion` |
| `DD_INSTALL_INFO_INSTALLER_VERSION` | `InstallerVersion` |

## Usage

### Inventory payloads

Every metadata component that sends an inventory payload calls `installinfo.Get` to include the
install method in the payload:

```go
// comp/metadata/inventoryagent/inventoryagentimpl/inventoryagent.go
installinfoGet = installinfo.Get  // package-level var for testability

info, err := installinfoGet(conf)
if err == nil {
    metadata["install_method_tool"] = info.Tool
    metadata["install_method_tool_version"] = info.ToolVersion
    metadata["install_method_installer_version"] = info.InstallerVersion
}
```

The same pattern is used in:

- `comp/metadata/host` — general host metadata
- `comp/metadata/clusteragent` — cluster agent metadata
- `comp/metadata/packagesigning` — package signing metadata

### Agent startup

`cmd/agent/subcommands/run/command.go` calls `LogVersionHistory()` on startup so the version
history file is updated whenever the agent is upgraded.

### Flare

`pkg/flare/archive.go` includes the `install_info` file in flare archives to aid support
investigations.

### Agent IPC API

`comp/api/api/apiimpl/internal/agent/agent.go` registers both HTTP handlers so the install info
can be read and updated at runtime without modifying files on disk.

## Notes

- `WriteInstallInfo` is idempotent on Linux: it is a no-op if `install_info` already exists.
  This ensures the file reflects the initial installation tool even if the agent is subsequently
  restarted or the configuration is regenerated.
- The runtime override (via `HandleSetInstallInfo`) is stored only in memory; it is lost on agent
  restart. It is intended for use by the Datadog Installer during the install transaction, not
  as a persistent configuration mechanism.
- All string fields pass through `scrubber.ScrubString` before being stored or returned to
  prevent accidental leakage of secrets that may appear in tool version strings.

## Cross-references

| Topic | See also |
|-------|----------|
| Default platform-specific file paths (including the `install_info` location relative to `datadog.yaml`) | [`pkg/util/defaultpaths`](defaultpaths.md) |
| Fleet Installer that calls `WriteInstallInfo` after a successful package install | [`pkg/fleet/installer`](../../pkg/fleet/installer.md) |
| Inventory agent component that reads `InstallInfo` and includes it in the `datadog_agent` payload | [`comp/metadata/inventoryagent`](../../../comp/metadata/inventoryagent.md) |
| HTTP API server that registers `HandleSetInstallInfo` and `HandleGetInstallInfo` | [`comp/api/api`](../../../comp/api/api.md) |
| Credential scrubbing applied to all InstallInfo fields before return | [`pkg/util/scrubber`](scrubber.md) |

### Where the `install_info` file lives

`GetFilePath` delegates to `pkg/util/defaultpaths` to determine the agent configuration directory. On Linux this resolves to `/etc/datadog-agent/install_info`; on macOS to `/opt/datadog-agent/etc/install_info`; on Windows to `%ProgramData%\Datadog\install_info`. The companion `install.json` file is always written to `/etc/datadog-agent/install.json` on Linux (hard-coded path, not configurable).

### Priority chain in context

```
HandleSetInstallInfo POST  ← highest: Datadog Installer sets this during the install
  │  (in-memory only; cleared on restart)
  ▼
DD_INSTALL_INFO_TOOL / _TOOL_VERSION / _INSTALLER_VERSION  ← env var override
  │  (all three must be set together; partial sets are rejected)
  ▼
install_info YAML file  ← lowest: written by the installation tool at install time
  │  path: GetFilePath(conf)  → via pkg/util/defaultpaths.GetDefaultConfPath()
  ▼
comp/metadata/inventoryagent  ← reads via installinfo.Get(conf)
  │  populates install_method_tool / _tool_version / _installer_version keys
  ▼
datadog_agent inventory payload  ← sent to Datadog backend
```
