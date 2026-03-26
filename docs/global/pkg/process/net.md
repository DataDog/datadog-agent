> **TL;DR:** `pkg/process/net` is a thin IPC client that lets the process-agent query the system-probe daemon over its Unix socket for privileged per-PID stats and the VPC network ID.

# pkg/process/net

## Purpose

`pkg/process/net` is a thin IPC client that lets the process-agent (or process check running inside the core agent) query the **system-probe** daemon over its Unix/named-pipe socket. It provides two HTTP-over-socket calls:

1. `GetProcStats` — fetch per-PID process stats (CPU, memory, open file descriptors, …) from system-probe's `ProcessModule`.
2. `GetNetworkID` — fetch the VPC/network ID from system-probe's `NetworkTracerModule` (used when the agent process cannot determine its own network namespace).

The package is intentionally small. All socket lifecycle management and URL construction is delegated to `pkg/system-probe/api/client` (`sysprobeclient`).

## Key elements

### Key functions

| Symbol | File / Build constraint | Description |
|--------|------------------------|-------------|
| `GetProcStats(client *http.Client, pids []int32) (*model.ProcStatsWithPermByPID, error)` | `common.go` — `linux \|\| windows` | POSTs a protobuf-encoded `ProcessStatRequest` to system-probe's `/api/v1/modules/process/stats` endpoint. Deserialises the response using the content-type returned by the server. |
| `GetNetworkID(client *http.Client) (string, error)` | `common_linux.go` — `linux` | GETs `/api/v1/modules/network_tracer/network_id` and returns the plain-text VPC ID. |
| *(stub)* `GetProcStats` | `common_unsupported.go` — `!linux && !windows` | Always returns `errors.New("unsupported platform")`. |

### Key interfaces

### Wire format

- Request: protobuf via `pkg/proto/pbgo/process.ProcessStatRequest`.
- Response: negotiated via `Accept`/`Content-type` headers. The unmarshaler is selected by `pkg/process/encoding.GetUnmarshaler(contentType)`.

### Configuration and build flags

The package compiles meaningful code only on `linux` and `windows`; a stub returns an error on other platforms. There are no agent config keys — the socket path is passed by the caller.

### HTTP client

The caller is responsible for providing an `*http.Client` already configured to speak over the system-probe Unix socket (or Windows named pipe). In practice this client is created by `pkg/system-probe/api/client.Get()` and reused for the lifetime of the check.

URL construction uses `sysprobeclient.ModuleURL(module, path)` which produces a URL of the form `http://sysprobe/<module>/<path>` that the underlying transport maps to the socket.

## Usage

### In the process check

`pkg/process/checks.ProcessCheck` calls `GetProcStats` in the realtime path to merge per-PID stats collected by system-probe (which has elevated privileges) with the process data collected by the agent itself:

```go
// pkg/process/checks/process_rt.go (simplified)
if p.sysprobeClient != nil && p.sysProbeConfig.ProcessModuleEnabled {
    mergeStatWithSysprobeStats(p.lastPIDs, procs, p.sysprobeClient)
}
```

`GetNetworkID` is called from `pkg/process/checks/net_linux.go` when the local network namespace check fails, falling back to system-probe which runs in the root namespace:

```go
networkID, err = net.GetNetworkID(sysProbeClient)
```

### Build constraints

The package only compiles meaningful code on Linux and Windows (where system-probe exists). On other platforms the exported symbols are no-ops that return an error, allowing callers to compile cross-platform without `//go:build` guards.

## Platform support

| Platform | `GetProcStats` | `GetNetworkID` |
|----------|---------------|----------------|
| Linux    | Yes           | Yes            |
| Windows  | Yes           | No             |
| macOS / other | Stub (error) | Not compiled |
