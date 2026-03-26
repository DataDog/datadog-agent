# Package `pkg/system-probe`

## Purpose

`pkg/system-probe` provides the interface layer between the Datadog Agent and
the `system-probe` daemon — a separate, privileged process that runs eBPF
programs and other kernel-level instrumentation.

The package is split into three concerns:

- **`pkg/system-probe/config`** — loads and validates `system-probe.yaml`,
  determines which modules are enabled, and exposes a `types.Config` struct that
  is shared across the whole codebase without pulling in the full system-probe
  build.
- **`pkg/system-probe/api`** — defines the module lifecycle (the `Module`
  interface and `Factory` type), the HTTP server infrastructure (listener,
  router, module loader), and the client library that other agent components use
  to query system-probe over a Unix socket.
- **`pkg/system-probe/utils`** — miscellaneous helpers used internally.

The `system-probe` binary itself lives in `cmd/system-probe/`. Each feature
(network monitoring, OOM kill, eBPF check, etc.) is a _module_ that is
registered at startup and exposes HTTP endpoints under its own path prefix.

### Architecture overview

```
cmd/system-probe
  └─ module.Register(cfg, httpMux, factories, ...)
       ├─ for each enabled factory:
       │    factory.Fn(cfg, deps) → Module
       │    Module.Register(Router)   ← registers /<module-name>/... endpoints
       └─ HTTP server listens on Unix socket (sysprobe_socket)

Other agent processes (agent, process-agent, ...)
  └─ pkg/system-probe/api/client
       └─ client.GetCheckClient() → CheckClient
            └─ CheckClient.GetCheck[T](module) → T   (HTTP GET /<module>/check)
```

---

## Key elements

### `pkg/system-probe/config` and `pkg/system-probe/config/types`

#### Types

| Type | Description |
|---|---|
| `types.Config` | The complete system-probe configuration. Passed to every module factory. |
| `types.ModuleName` | `string` alias used as a typed module identifier throughout the codebase. |

#### `types.Config` fields

| Field | Description |
|---|---|
| `Enabled` | Whether system-probe is enabled at all. |
| `EnabledModules` | `map[ModuleName]struct{}` — set of modules that should be started. |
| `ExternalSystemProbe` | True when system-probe runs in a separate container; disables the embedded copy. |
| `SocketAddress` | Path to the Unix socket (default from `system_probe_config.sysprobe_socket`). |
| `MaxConnsPerMessage` | Maximum number of connections per network message. |
| `LogFile` / `LogLevel` | Logging configuration. |
| `DebugPort` / `HealthPort` | Optional pprof / health HTTP ports. |
| `TelemetryEnabled` | Enables Prometheus-style internal telemetry. |

#### Module name constants (defined in `config/config.go`)

| Constant | Value | Enabled when |
|---|---|---|
| `NetworkTracerModule` | `network_tracer` | NPM, USM, CCM, or CSM network monitoring is on. |
| `OOMKillProbeModule` | `oom_kill_probe` | `system_probe_config.enable_oom_kill = true` |
| `TCPQueueLengthTracerModule` | `tcp_queue_length_tracer` | `system_probe_config.enable_tcp_queue_length = true` |
| `ProcessModule` | `process` | `system_probe_config.process_config.enabled = true` |
| `EventMonitorModule` | `event_monitor` | CSM, FIM, USM event stream, GPU, or DI is on. |
| `DynamicInstrumentationModule` | `dynamic_instrumentation` | DI feature is enabled. |
| `EBPFModule` | `ebpf` | `ebpf_check.enabled = true` |
| `LanguageDetectionModule` | `language_detection` | `system_probe_config.language_detection.enabled = true` |
| `PingModule` | `ping` | `ping.enabled = true` |
| `TracerouteModule` | `traceroute` | `traceroute.enabled = true` |
| `DiscoveryModule` | `discovery` | Discovery feature is enabled. |
| `GPUMonitoringModule` | `gpu` | GPU monitoring is enabled. |
| `ComplianceModule` | `compliance` | Compliance module conditions are met. |
| `NoisyNeighborModule` | `noisy_neighbor` | Noisy neighbor feature is enabled. |

#### Key functions

| Function | Description |
|---|---|
| `config.New(configPath, fleetPoliciesDirPath)` | Loads `system-probe.yaml` (or the provided path), applies fleet policies, runs `Adjust`, and returns `*types.Config`. |
| `config.Adjust(cfg)` | Applies deprecation migrations and cross-field inferences (e.g. backwards-compat for `system_probe_config.enabled`). Idempotent. |
| `(c Config) ModuleIsEnabled(name)` | Returns true if `name` is in `EnabledModules`. |

---

### `pkg/system-probe/api/module`

This package owns the module contract and the global loader singleton.

#### Types

| Type | Description |
|---|---|
| `Module` | Interface every system-probe module must implement: `GetStats() map[string]interface{}`, `Register(*Router) error`, `Close()`. |
| `Factory` | Bundles a `Name`, a constructor `Fn(cfg, deps) (Module, error)`, and (Linux only) eBPF requirements (`NeedsEBPF`, `OptionalEBPF`). |
| `FactoryDependencies` | `fx.In` struct injected into every factory: `SysprobeConfig`, `CoreConfig`, `Log`, `WMeta`, `Tagger`, `Telemetry`, `Statsd`, `Hostname`, `Traceroute`, `ConnectionsForwarder`, etc. |
| `Router` | Wraps `gorilla/mux` to support re-registration of handlers after a module restart. Routes are registered under `/<module-name>/...`. |

#### Key functions

| Function | Description |
|---|---|
| `module.Register(cfg, httpMux, factories, rcclient, deps)` | Initializes all enabled modules, registers their HTTP endpoints, and starts per-module stats goroutines. Returns an error if _no_ module loaded successfully. |
| `module.RestartModule(factory, deps)` | Tears down a running module, re-creates it from the factory, and re-registers its routes without restarting the HTTP server. |
| `module.GetStats()` | Returns a `map[string]any` of stats from all running modules, keyed by module name. |
| `module.Close()` | Calls `Close()` on every module and unregisters their routes. |
| `ErrNotEnabled` | Sentinel error a `Factory.Fn` may return to indicate the module should not start (treated as a soft failure — system-probe continues with other modules). |

#### Module lifecycle

```
Register()
  ├─ preRegister()   — e.g. eBPF subsystem setup on linux_bpf
  ├─ factory.Fn()    → Module
  ├─ Module.Register(Router)
  └─ postRegister()  — e.g. BTF flush, lock-contention collector

RestartModule()
  ├─ router.Unregister()
  ├─ module.Close()
  ├─ factory.Fn()    → new Module
  └─ new Module.Register(same Router)

Close()
  └─ module.Close() for each module
```

---

### `pkg/system-probe/api/client`

Client library for agent-side components to query system-probe over its Unix
socket.

#### Types

| Type | Description |
|---|---|
| `CheckClient` | HTTP client pre-configured to dial the system-probe socket. Manages startup detection. |
| `CheckClientOption` | Functional option for `GetCheckClient`: `WithSocketPath`, `WithCheckTimeout`, `WithStartupCheckTimeout`. |

#### Key functions

| Function | Description |
|---|---|
| `client.Get(socketPath)` | Returns a memoized `*http.Client` that dials `socketPath`. For low-level use. |
| `client.GetCheckClient(...CheckClientOption)` | Returns a `*CheckClient` with startup detection and per-request telemetry. The preferred entry point for checks. |
| `client.GetCheck[T](client, module)` | Generic helper: `GET /<module>/check`, JSON-decodes the response into `T`. |
| `client.Post[T](client, endpoint, body, module)` | Generic helper: `POST /<module>/<endpoint>` with optional JSON body. |
| `client.ModuleURL(module, endpoint)` | Constructs `http://sysprobe/<module>/<endpoint>`. |
| `client.URL(endpoint)` | Constructs a URL for module-less endpoints (e.g. `/debug/stats`). |
| `client.IgnoreStartupError(err)` | Returns nil if `err` is `ErrNotStartedYet` (suppresses check errors during system-probe startup). |
| `client.ReadAllResponseBody(resp)` | Reads the full response body with pre-allocation when `Content-Length` is known. |

#### Sentinel errors

| Error | Meaning |
|---|---|
| `ErrNotImplemented` | system-probe is not supported on this OS. |
| `ErrNotStartedYet` | system-probe has not responded yet; within the startup window. Callers should wrap with `IgnoreStartupError`. |

---

### `pkg/system-probe/api/server`

Provides the `net.Listener` factory (`listener_unix.go`, `listener_windows.go`,
`listener_others.go`) that the system-probe HTTP server binds to. On Linux and
macOS it uses a Unix domain socket; on Windows it uses a named pipe.

`server.ErrNotImplemented` is returned on unsupported platforms.

---

## Usage

### Implementing a new module

1. Create a `Factory` with a constructor that returns a type implementing
   `module.Module`:

```go
// build tag: linux
var Factory = &module.Factory{
    Name:      config.TCPQueueLengthTracerModule,
    NeedsEBPF: func() bool { return true },
    Fn: func(cfg *types.Config, deps module.FactoryDependencies) (module.Module, error) {
        if !cfg.ModuleIsEnabled(config.TCPQueueLengthTracerModule) {
            return nil, module.ErrNotEnabled
        }
        return newTCPQueueLengthModule(cfg, deps)
    },
}
```

2. Implement `Module.Register(*module.Router)` to expose HTTP endpoints:

```go
func (m *myModule) Register(r *module.Router) error {
    r.HandleFunc("/check", m.handleCheck)
    return nil
}
```

3. Register the factory in `cmd/system-probe/modules/` so it is passed to
   `module.Register` at startup.

### Querying system-probe from a check

```go
import (
    sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
    sysconfig       "github.com/DataDog/datadog-agent/pkg/system-probe/config"
    pkgconfigsetup  "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// In Configure():
socketPath := pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
t.sysProbeClient = sysprobeclient.GetCheckClient(
    sysprobeclient.WithSocketPath(socketPath),
)

// In Run():
stats, err := sysprobeclient.GetCheck[MyModuleStats](
    t.sysProbeClient,
    sysconfig.TCPQueueLengthTracerModule,
)
if err != nil {
    return sysprobeclient.IgnoreStartupError(err)
}
```

### Loading configuration

```go
import sysprobecfg "github.com/DataDog/datadog-agent/pkg/system-probe/config"

cfg, err := sysprobecfg.New("/etc/datadog-agent/system-probe.yaml", "")
if err != nil {
    // handle
}
if cfg.ModuleIsEnabled(sysprobecfg.NetworkTracerModule) {
    // network tracing is on
}
```

### Build tags

`pkg/system-probe/api/module` uses build-tag-gated files for platform
differences:

| File | Tag | Purpose |
|---|---|---|
| `factory_linux.go` | `linux` | `Factory` struct with `NeedsEBPF`/`OptionalEBPF` fields. |
| `factory_others.go` | everything else | Stub `Factory` without eBPF fields. |
| `loader_linux.go` | `linux_bpf` | `preRegister`/`postRegister` call `ebpf.Setup`. |
| `loader_unsupported.go` | non-linux | `preRegister`/`postRegister` are no-ops. |
| `loader_windows.go` | `windows` | Windows-specific registration hooks. |

The `config/` package similarly has `config_linux_bpf.go`, `config_darwin.go`,
`config_windows.go`, and `config_unsupported.go` for platform-specific socket
paths and validation.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/core/sysprobeconfig` | [sysprobeconfig.md](../comp/core/sysprobeconfig.md) | fx-injectable wrapper around `system-probe.yaml`. It calls `sysconfig.New()` internally and exposes both a `model.ReaderWriter` and `SysProbeObject() *types.Config`. Use this component (instead of calling `config.New()` directly) inside fx apps. Passed to every module factory via `FactoryDependencies.SysprobeConfig`. |
| `pkg/ebpf` | [ebpf.md](ebpf.md) | Shared eBPF infrastructure consumed by every eBPF-backed module. On Linux, `module.Register` calls `ebpf.Setup(cfg, rcClient)` in `preRegister` and `ebpf.FlushBTF()` in `postRegister`. The `Factory.NeedsEBPF` / `OptionalEBPF` fields declared by each module factory drive the eBPF readiness check performed before loading. |
| `pkg/network` | [network/network.md](network/network.md) | Primary consumer of `pkg/system-probe/api`. The `NetworkTracerModule` exposes `GET /network_tracer/connections` (encoded via `pkg/network/encoding`) over the Unix socket; agent-side checks query it with `client.GetCheck[network.Connections]`. The network tracer config extends `ebpf.Config` and reads `system_probe_config.*` keys parsed by `comp/core/sysprobeconfig`. |
| `pkg/security/probe` | [security/probe.md](security/probe.md) | CWS (Cloud Workload Security) eBPF probe. Loaded as the `EventMonitorModule`. `EBPFProbe` uses `pkg/ebpf.Manager`, `telemetry.ErrorsTelemetryModifier`, and BTF constant-fetchers — all of which depend on `ebpf.Setup` having been called in `preRegister`. |
| `comp/remote-config/rcclient` | [../comp/remote-config/rcclient.md](../comp/remote-config/rcclient.md) | RC client injected into `module.Register` so that modules can subscribe to RC products (e.g. `ProductAPMSampling`, `ProductAgentConfig`). When `IsSystemProbe: true`, the RC client routes `AGENT_CONFIG` log-level changes to `comp/core/sysprobeconfig` rather than the main agent config. |
