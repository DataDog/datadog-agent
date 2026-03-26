# pkg/util/net

## Purpose

`pkg/util/net` provides small, focused network utilities used across the agent.
It currently covers two concerns:

1. **FQDN resolution** — converting a short hostname to its fully qualified domain name via a DNS forward-then-reverse lookup.
2. **Unix Domain Socket availability** — checking whether a UDS datagram socket exists on disk and is writable, which is the pre-flight check before DogStatsD connects over UDS.

The package is intentionally thin: it wraps Go's standard `net` package and `golang.org/x/sys/unix` to expose OS-specific behaviour behind a stable API.

## Key elements

| Symbol | File(s) | Description |
|--------|---------|-------------|
| `Fqdn(hostname string) string` | `host.go` / `host_serverless.go` | Resolves a hostname to its FQDN using a forward (`LookupIP`) and reverse (`LookupAddr`) DNS round-trip. Falls back to the original hostname on any error. Under the `serverless` build tag the function always returns `""` because DNS is unavailable in that environment. |
| `IsUDSAvailable(path string) bool` | `endpoint_check_nix.go` / `endpoint_check_windows.go` | Returns `true` only when the path exists, is a socket (`ModeSocket`), and the process has write permission (`unix.W_OK`). Always returns `false` on Windows, where DogStatsD does not use UDS datagram sockets. |

### Build tags

| Tag | Effect |
|-----|--------|
| `serverless` | `Fqdn` returns `""` unconditionally (no DNS in Lambda) |
| `windows` | `IsUDSAvailable` returns `false` unconditionally |

## Usage

### Hostname utilities

`Fqdn` is used by the core host-metadata pipeline (`comp/metadata/host/hostimpl/utils/meta.go`) to enrich host metadata with the fully-qualified name when only a short hostname is known.

```go
import utilnet "github.com/DataDog/datadog-agent/pkg/util/net"

fqdn := utilnet.Fqdn(shortHostname)
```

### DogStatsD UDS check

`IsUDSAvailable` is called before the agent tries to connect to a DogStatsD UDS socket, for example in `pkg/jmxfetch/jmxfetch.go` and the CloudFoundry network check helpers. It prevents connection attempts to paths that are missing, are not sockets, or are not yet writable.

```go
import utilnet "github.com/DataDog/datadog-agent/pkg/util/net"

if utilnet.IsUDSAvailable("/var/run/datadog/dsd.socket") {
    // safe to connect
}
```

## Related packages

| Package / component | Relationship |
|---|---|
| [`pkg/util/http`](http.md) | Provides the shared `http.Transport` used for all TCP-based outbound connections in the agent. `pkg/util/net` handles the pre-connection UDS availability check, while `pkg/util/http` handles TLS, proxy, and connection-management concerns for TCP connections. |
| [`comp/dogstatsd/listeners`](../../comp/dogstatsd/listeners.md) | The UDS datagram and stream listeners (`UDSDatagramListener`, `UDSStreamListener`) own the server side of DogStatsD UDS sockets. `IsUDSAvailable` is used on the *client* side (e.g., JMXFetch, CloudFoundry helpers) to check that those sockets exist and are writable before attempting a connection. |
| [`comp/core/ipc`](../../comp/core/ipc.md) | The IPC component also communicates over a Unix domain socket (`agent_ipc.socket_path`), but it manages its own socket lifecycle and TLS authentication independently of `pkg/util/net`. `pkg/util/net` is not used for IPC socket checks; those paths are handled internally by the IPC transport layer. |
