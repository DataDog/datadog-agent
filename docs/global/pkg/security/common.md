> **TL;DR:** `pkg/security/common` is a lightweight cross-cutting utilities package shared by both the `security-agent` and `system-probe` sides of CWS, providing the `RawReporter` interface, log-context helpers for intake routing, address utilities, and passwd/group parsers.

# pkg/security/common — shared CWS types and utilities

## Purpose

`pkg/security/common` is a **cross-cutting utilities package** used by both the `security-agent` and `system-probe` sides of CWS (and by the compliance engine). It avoids import cycles by housing types and helpers that multiple CWS sub-packages need without pulling in heavy dependencies. The package has no build tag and compiles on all platforms.

Sub-package `usergrouputils/` provides portable `/etc/passwd` and `/etc/group` parsers used by the ptracer.

## Key elements

### Key interfaces

#### `RawReporter` interface (`raw_reporter.go`)

```go
type RawReporter interface {
    ReportRaw(content []byte, service string, timestamp time.Time, tags ...string)
}
```

The central abstraction for sending CWS events to the intake. Implemented by `reporter.RuntimeReporter`; consumed throughout `pkg/security/module`, `pkg/security/agent`, and `pkg/compliance`.

### Key functions

#### Log context helpers (`logs_context.go`)

These functions build the `logsconfig.Endpoints` and `client.DestinationsContext` pair needed to route events to the correct Datadog intake track.

| Function | Track type | Config prefix | Endpoint prefix |
|----------|-----------|---------------|-----------------|
| `NewLogContextRuntime(useSecRuntimeTrack)` | `SecRuntime` or `Logs` | `runtime_security_config.endpoints.` | `runtime-security-http-intake.logs.` |
| `NewLogContextSecInfo()` | `SecInfo` | `runtime_security_config.endpoints.` | `runtime-security-http-intake.logs.` |
| `NewLogContextCompliance()` | `compliance` | `compliance_config.endpoints.` | `cspm-intake.` |
| `NewLogContext(logsConfig, endpointPrefix, trackType, origin, protocol)` | configurable | configurable | configurable |

`NewLogContextRuntime` is gated by the `runtime_security_config.use_secruntime_track` config key: when `false` (the default), events are routed over the `Logs` track; when `true`, the dedicated `SecRuntime` track is used. `StartRuntimeSecurity` in `pkg/security/agent` reads this key and calls `NewLogContextRuntime` accordingly.

Track type constants (`TrackType = logsconfig.IntakeTrackType`):

| Constant | Value |
|----------|-------|
| `SecRuntime` | `"secruntime"` |
| `Logs` | `"logs"` |
| `SecInfo` | `"secinfo"` |
| `cwsIntakeOrigin` (unexported) | `"cloud-workload-security"` |

#### Cloud provider account ID (`account_id.go`)

```go
func QueryAccountIDTag() string
```

Detects EC2 (`account_id`), GCE (`project_id`), or Azure (`subscription_id`) and returns a `"<name>:<value>"` tag string, lazily computed and cached via `sync.Once`. Used to enrich CWS events with the cloud account context.

#### Address utilities (`address_utils.go`)

```go
func GetFamilyAddress(path string) string   // "unix" or "tcp"
func GetCmdSocketPath(socketPath, cmdSocketPath string) (string, error)
```

`GetCmdSocketPath` derives the command socket path from the event socket path: for Unix sockets it prepends `cmd-` to the filename; for TCP it increments the port by 1. Used to wire `system-probe` ↔ `security-agent` IPC.

### Key types

#### Static hostname service (`hostname_service.go`)

```go
type StaticHostnameService struct { ... }
func NewStaticHostnameService(hostname string) *StaticHostnameService
```

Implements `hostnameinterface.Component` with a fixed hostname. Used by `reporter.newReporter` to inject the agent hostname into the logs pipeline without requiring a full hostname resolution component.

#### Container filter (`filters.go`)

```go
func NewContainerFilter(cfg model.Config, prefix string) (*containers.Filter, error)
```

Builds an include/exclude container filter from config keys `<prefix>container_include`, `<prefix>container_exclude`, and optionally appends the pause container exclusion list when `<prefix>exclude_pause_container` is true.

#### No-op status provider (`status_provider.go`)

```go
type NoopStatusProvider struct{}
func (n *NoopStatusProvider) AddGlobalWarning(string, string) {}
func (n *NoopStatusProvider) RemoveGlobalWarning(string) {}
```

Satisfies the `pipeline.StatusProvider` interface without side effects. Used by `reporter.newReporter` when no real status reporting is needed.

#### `usergrouputils/` — passwd/group parsers

| Function | Description |
|----------|-------------|
| `ParsePasswdFile(r io.Reader) (map[int]string, error)` | Parses an `/etc/passwd`-format stream into `uid → username`. |
| `ParsePasswd(fs fs.FS, path string) (map[int]string, error)` | Opens and parses a passwd file from an `fs.FS`. |
| `ParseGroupFile(r io.Reader) (map[int]string, error)` | Parses an `/etc/group`-format stream into `gid → groupname`. |
| `ParseGroup(fs fs.FS, path string) (map[int]string, error)` | Opens and parses a group file from an `fs.FS`. |

These are used by the ptracer (`pkg/security/ptracer`) to resolve UIDs/GIDs to human-readable names when populating exec credentials in syscall messages.

## Usage

`pkg/security/common` is imported by roughly a dozen packages across the security and compliance subsystems. Typical patterns:

**Setting up the intake pipeline for the security-agent:**

```go
endpoints, ctx, _ := common.NewLogContextRuntime(useSecRuntimeTrack)
reporter, _ := reporter.NewCWSReporter(hostname, stopper, endpoints, ctx, compression)
```

**Deriving the command socket path:**

```go
cmdSocket, err := common.GetCmdSocketPath(
    cfg.GetString("runtime_security_config.socket"),
    cfg.GetString("runtime_security_config.cmd_socket"),
)
family := common.GetFamilyAddress(cmdSocket)
```

**Enriching events with cloud account tag:**

```go
tag := common.QueryAccountIDTag() // "account_id:123456789"
```

**Routing `SecInfo` events in `RuntimeSecurityAgent`:**

`DispatchEvent` in `pkg/security/agent` checks `evt.Track == string(common.SecInfo)` to decide whether to call `secInfoReporter.ReportRaw` (for remediation-status events) or `reporter.ReportRaw` (for all other CWS events).

## Related documentation

| Doc | Description |
|-----|-------------|
| [security.md](security.md) | Top-level CWS overview: `RawReporter` and the log reporter created by `reporter.NewCWSReporter` sit between `RuntimeSecurityAgent` and the Datadog backend. |
| [probe.md](probe.md) | `probe.go` references `common.NewLogContextRuntime` indirectly via `StartRuntimeSecurity`; the probe origin string (`"ebpf"`, `"ebpfless"`) can appear in events enriched with `QueryAccountIDTag`. |
| [agent.md](agent.md) | `StartRuntimeSecurity` calls `common.NewLogContextRuntime` and `common.NewLogContextSecInfo` to set up the two reporter pipelines; `DispatchEvent` uses `common.SecInfo` for routing. |
| [../../pkg/util/scrubber.md](../../pkg/util/scrubber.md) | The `reporter.NewCWSReporter` pipeline that `RawReporter` feeds into uses a `Scrubber` to clean events before transmission; `pkg/security/common` itself does not depend on the scrubber directly. |
