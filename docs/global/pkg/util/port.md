# pkg/util/port

**Import paths:**
- `github.com/DataDog/datadog-agent/pkg/util/port`
- `github.com/DataDog/datadog-agent/pkg/util/port/portlist`

## Purpose

`pkg/util/port` (and its sub-package `portlist`) provides cross-platform enumeration of the TCP and UDP ports currently listening on the local machine, together with the name and PID of the owning process where available. It is used by the agent's diagnose suite to verify that the ports required by the agent's configuration are in use by the expected agent processes.

The implementation is adapted from [Tailscale's portlist package](https://github.com/tailscale/tailscale) and uses OS-native sources:

| Platform | Data source |
|---|---|
| Linux | `/proc/net/tcp`, `/proc/net/tcp6`, `/proc/net/udp`, `/proc/net/udp6` + `/proc/<pid>/fd` for process names |
| macOS | `netstat -n` output |
| Windows | `GetExtendedTcpTable` / `GetExtendedUdpTable` Win32 APIs |
| Other | Not implemented; `Poll` returns an error |

## Key Elements

### `portlist.Port`

```go
type Port struct {
    Proto   string // "tcp" or "udp"
    Port    uint16
    Process string // optional: process name if readable
    Pid     int    // optional: process PID if readable (requires appropriate permissions)
}
```

Also exported as `port.Port` (a type alias) at the top-level package.

### `portlist.List`

`type List []Port` — a slice of ports with an `equal` helper used by `Poller` for change detection.

### `portlist.Poller`

The core type for port enumeration:

```go
type Poller struct {
    IncludeLocalhost bool
    // ... unexported fields
}

func (p *Poller) Poll() (ports []Port, changed bool, err error)
```

- `IncludeLocalhost` — when true, includes services bound only to `127.0.0.1`/`::1`.
- `Poll()` — returns the current list of listening ports. On the first call it initialises the OS-specific backend. On subsequent calls it returns `changed = false` (and `ports = nil`) if the list is identical to the previous snapshot, which lets callers cheaply poll in a loop.

### `port.GetUsedPorts() ([]Port, error)`

Convenience function in the top-level package. Creates a `Poller` with `IncludeLocalhost: true` and returns the full current port list, discarding the change-detection return value.

### `portlist.osImpl` interface

```go
type osImpl interface {
    AppendListeningPorts(base []Port) ([]Port, error)
}
```

The internal platform abstraction. Each platform file (`poller_linux.go`, `poller_darwin.go`, `poller_windows.go`) provides an `init()` method on `Poller` that sets `p.os` to the appropriate implementation.

## Usage

### Diagnose suite (`pkg/diagnose/ports/ports.go`)

`DiagnosePortSuite` calls `port.GetUsedPorts()` to get the current listening ports, then checks each configuration key whose name ends in `_port` or starts with `port_`:

```go
ports, err := port.GetUsedPorts()
// ...
portMap := make(map[uint16]port.Port)
for _, p := range ports {
    portMap[p.Port] = p
}

// For each config key matching a port pattern:
if usedPort, ok := portMap[uint16(configValue)]; ok {
    // Verify usedPort.Process is one of the known agent process names.
}
```

Diagnoses are reported as `Success` (port used by a known agent process), `Warning` (port used but PID unknown — possibly a different user's process), or `Fail` (port used by a non-agent process).

## Cross-references

| Topic | See also |
|-------|----------|
| Diagnose framework — suite registration, `Diagnosis` types, and how `DiagnosePortSuite` is wired into `datadog-agent diagnose` | [pkg/diagnose](../diagnose/diagnose.md) |
| `comp/core/diagnose` — the FX component and global suite catalog that consumes `DiagnosePortSuite` | [comp/core/diagnose](../../comp/core/diagnose.md) |
| Flare integration — diagnose output (including port checks) is attached as `diagnose.log` to every flare | [comp/core/flare](../../comp/core/flare.md) |
