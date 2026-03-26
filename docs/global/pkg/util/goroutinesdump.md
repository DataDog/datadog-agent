> **TL;DR:** Fetches the full goroutine stack trace of the running agent by hitting its built-in pprof HTTP endpoint over the IPC address, used as a shutdown-timeout diagnostic by the logs agent.

# pkg/util/goroutinesdump

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/goroutinesdump`

## Purpose

`pkg/util/goroutinesdump` provides a single helper function to retrieve the full goroutine stack trace of a running Agent process. It does so by hitting the Agent's built-in `pprof` HTTP endpoint (`/debug/pprof/goroutine?debug=2`) over the IPC address. This makes it usable from any component or sub-process that can reach the Agent's internal HTTP server, without requiring direct access to the Agent process or the `runtime` package.

The primary use case is graceful-shutdown diagnostics: if a component times out waiting for the Agent to stop, it can call `Get()` and log the dump to help identify which goroutines are blocking.

## Key Elements

### `Get() (string, error)`

Fetches and returns the goroutine dump as a plain-text string. The IPC address and `expvar_port` are read from the agent's configuration (`pkgconfigsetup.Datadog()`). A 2-second HTTP timeout is applied.

Returns an error if:
- The IPC address cannot be resolved from config.
- The HTTP request fails or times out.
- The response body cannot be read.

## Usage

### Logs Agent shutdown (`comp/logs/agent/agentimpl/agent.go`)

The logs agent calls `Get()` when a graceful shutdown exceeds a 5-second timeout, logging the result as a warning to help diagnose stuck goroutines:

```go
timeout := time.NewTimer(5 * time.Second)
select {
case <-c:
case <-timeout.C:
    a.log.Warn("Force close of the Logs Agent, dumping the Go routines.")
    if stack, err := goroutinesdump.Get(); err != nil {
        a.log.Warnf("can't get the Go routines dump: %s\n", err)
    } else {
        a.log.Warn(stack)
    }
}
```

### Flare integration (`pkg/flare/archive.go`)

`pkg/flare`'s `RemoteFlareProvider.GetGoRoutineDump()` fetches goroutine stacks via the same `expvar_port` pprof endpoint (using a direct HTTP call rather than this package). The resulting dump is included in every flare archive as a diagnostic snapshot of the running agent. `goroutinesdump.Get()` and `RemoteFlareProvider.GetGoRoutineDump()` target the same endpoint but differ in how they resolve the address — this package uses `GetIPCAddress` (IPC address + `expvar_port`), whereas the flare provider uses `127.0.0.1` + `expvar_port` directly.

### Notes

- The function requires the Agent's HTTP server to be running and reachable. It is not usable before the server starts or after it stops.
- The output format is the standard Go pprof text format (`debug=2`), which includes goroutine IDs, states, and full stack frames.
- For programmatic consumption consider using Go's `runtime.Stack` or `net/http/pprof` directly when the pprof HTTP server is not available (e.g. in tests or early-boot code).

## Cross-references

| Topic | See also |
|-------|----------|
| `pkg/util/log` — the logging package used to emit the goroutine dump as a warning on shutdown timeout | [pkg/util/log](log.md) |
| Flare component — goroutine dumps are included in flare archives via `RemoteFlareProvider.GetGoRoutineDump()` | [comp/core/flare](../../comp/core/flare.md) |
| `comp/logs/agent` — the only direct caller of `goroutinesdump.Get()`; invoked on graceful-shutdown timeout | [`comp/logs/agent/agentimpl/agent.go`](../../../../comp/logs/agent/agentimpl/agent.go) |
