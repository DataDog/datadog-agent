# comp/core/hostname — Hostname Resolution Component

**Team:** agent-runtimes
**Import path (interface):** `github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface`
**Import path (alias):** `github.com/DataDog/datadog-agent/comp/core/hostname` (re-exports the interface)

## Purpose

The hostname component exposes a single, consistent way to obtain the agent's
hostname. Hostname detection is non-trivial: the agent tries a prioritized list
of providers (config file, Fargate, GCE, Azure, FQDN, container metadata, OS,
EC2) and caches the result. The component wraps this logic so that every part
of the codebase queries it through the same interface, regardless of whether it
runs in the core agent, a remote sub-agent, or a serverless context.

## Key elements

### Component interface (`hostnameinterface/component.go`)

```go
type Component interface {
    Get(ctx context.Context) (string, error)
    GetWithProvider(ctx context.Context) (Data, error)
    GetSafe(ctx context.Context) string
}
```

| Method | Behavior |
|---|---|
| `Get` | Returns the hostname. Errors if resolution fails entirely. |
| `GetWithProvider` | Returns both the hostname and the name of the provider that resolved it (e.g. `"gce"`, `"aws"`, `"config"`, `"remote"`). |
| `GetSafe` | Same as `Get`, but returns `"unknown host"` instead of propagating an error. Safe to call during startup before logging is fully initialized. |

### Data type (`hostnameinterface/component.go`)

```go
type Data struct {
    Hostname string
    Provider string
}
```

Returned by `GetWithProvider`. The `Provider` field identifies which source
resolved the hostname — useful for diagnostics.

### Provider resolution order (`pkg/util/hostname/providers.go`)

The underlying `pkg/util/hostname.Get` function tries providers in order,
stopping at the first successful `stopIfSuccessful` provider:

1. `hostname` config key or `DD_HOSTNAME` env var
2. `hostname_file` config key
3. Fargate / sidecar (strips the hostname)
4. GCE instance metadata
5. Azure instance metadata
6. FQDN
7. Container metadata (kubelet, Docker, kube API server)
8. OS hostname
9. EC2 instance identity document

The result is cached in memory after the first successful resolution; subsequent
calls return the cached value immediately.

### Implementations

| Package | Module function | When to use |
|---|---|---|
| `hostnameimpl` | `hostnameimpl.Module()` | Core agent and any process that resolves the hostname locally. Delegates to `pkg/util/hostname`. |
| `remotehostnameimpl` | `remotehostnameimpl.Module()` | Sub-agents (process-agent, security-agent, etc.) that want to use the hostname already determined by the core agent. Calls the core agent's gRPC `GetHostname` endpoint (via `ipc.Component` for TLS), with up to 6 retries. Falls back to local resolution if the core agent is unreachable. Caches the result for 15 minutes. |
| `hostnameimpl` (serverless variant) | via build tag | Used in the serverless agent; resolves hostname from the Lambda function name. |

### fx wiring

Both `hostnameimpl.Module()` and `remotehostnameimpl.Module()` register their
constructor with `fx.Provide`. They satisfy `hostname.Component` (which is a
type alias for `hostnameinterface.Component`).

### Mock (`hostnameinterface/component_mock.go`, build tag `test`)

The `Mock` interface embeds `Component` without adding extra methods. Provide a
custom implementation in tests by satisfying `hostname.Component` directly, or
use `fxutil.Test` with a manually constructed service.

A common test pattern is to inline a minimal struct:

```go
type mockHostname struct{}
func (m *mockHostname) Get(_ context.Context) (string, error) { return "test-host", nil }
func (m *mockHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
    return hostnameinterface.Data{Hostname: "test-host", Provider: "mock"}, nil
}
func (m *mockHostname) GetSafe(_ context.Context) string { return "test-host" }
```

## Usage

### Core agent / local resolution

```go
// In an fx component that needs the hostname:
type deps struct {
    fx.In
    Hostname hostname.Component
}

func (d deps) reportHostname(ctx context.Context) {
    h, err := d.Hostname.Get(ctx)
    if err != nil {
        log.Warnf("hostname unavailable: %v", err)
        return
    }
    // use h
}
```

For non-critical paths where you just want a best-effort value:

```go
h := d.Hostname.GetSafe(ctx) // never returns an error
```

### Remote / sub-agent context

Wire `remotehostnameimpl.Module()` instead of `hostnameimpl.Module()` in the
fx application. The component still satisfies the same `hostname.Component`
interface; callers do not change.

### With provider information

```go
data, err := d.Hostname.GetWithProvider(ctx)
if err == nil {
    log.Infof("hostname=%s provider=%s", data.Hostname, data.Provider)
}
```

### Where it is wired in the agent

- `cmd/agent/subcommands/run/command.go` — core agent daemon uses `hostnameimpl.Module()`.
- `cmd/process-agent/command/main_common.go` — process-agent uses `remotehostnameimpl.Module()`.
- `cmd/security-agent`, `cmd/cluster-agent`, `cmd/otel-agent` — use either
  local or remote impl depending on the process role.
- `comp/privateactionrunner` — uses `hostname.Component` to tag action results.
- `pkg/process/checks/host_info.go` — uses `hostname.Component` to populate
  host-level metadata in process payloads.

## Relationship to pkg/util/hostname

`comp/core/hostname` wraps `pkg/util/hostname` and provides it through the fx component system:

| Layer | Package | Role |
|---|---|---|
| Resolution logic | `pkg/util/hostname` | Full provider chain, in-process cache, drift detection, expvar registration |
| Validation helpers | `pkg/util/hostname/validate` | `ValidHostname`, `NormalizeHost`, `CleanHostnameDir` |
| fx component | `comp/core/hostname/hostnameimpl` | Delegates `Get`/`GetWithProvider`/`GetSafe` to `pkg/util/hostname.GetWithProvider` |
| Remote fx component | `comp/core/hostname/remotehostnameimpl` | Calls the core agent's gRPC `GetHostname` endpoint (TLS from `ipc.Component`); falls back to local resolution; caches for 15 minutes |

See [`pkg/util/hostname`](../../pkg/util/hostname.md) for the full provider chain, EC2 special logic, drift detection metrics (`hostname.drift_detected`, `hostname.drift_resolution_time_ms`), and the `isOSHostnameUsable` heuristic.

### Config keys read

`hostnameimpl` delegates to `pkg/util/hostname`, which reads the following keys from [`comp/core/config`](config.md):

| Config key | Effect |
|---|---|
| `hostname` / `DD_HOSTNAME` | Highest-priority explicit hostname |
| `hostname_file` | Path to a file whose contents become the hostname |
| `hostname_fqdn` | Enable the `fqdn` provider (gated; false by default) |
| `ec2_prioritize_instance_id_as_hostname` | Force the EC2 provider to win over the OS hostname |
| `hostname_drift_initial_delay` / `hostname_drift_recurring_interval` | Timing of background drift detection |

### ipc dependency in remotehostnameimpl

`remotehostnameimpl` depends on [`comp/core/ipc`](ipc.md) to obtain the mutual-TLS client config for the gRPC call to the core agent. If the core agent is unreachable after 6 retries, it falls back to calling `pkg/util/hostname.Get` locally.

### fxutil integration

For tests that need a hostname, inline a minimal mock struct rather than wiring an entire `hostnameimpl`:

```go
type mockHostname struct{}
func (m *mockHostname) Get(_ context.Context) (string, error) { return "test-host", nil }
func (m *mockHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
    return hostnameinterface.Data{Hostname: "test-host", Provider: "mock"}, nil
}
func (m *mockHostname) GetSafe(_ context.Context) string { return "test-host" }

// In the test:
fxutil.Test[MyComp](t, fx.Options(
    fx.Provide(func() hostname.Component { return &mockHostname{} }),
    mycomp.Module(),
))
```

See [`pkg/util/fxutil`](../../pkg/util/fxutil.md) for the full test helper API.
