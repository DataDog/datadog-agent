> **TL;DR:** `pkg/eventmonitor` is the kernel event monitoring framework for system-probe that wraps the security probe and exposes a stable subscription API so non-security subsystems can receive kernel-level process lifecycle events (fork, exec, exit) without depending on the security module.

# pkg/eventmonitor

## Purpose

`pkg/eventmonitor` is the kernel event monitoring framework for `system-probe`. It wraps the security probe (`pkg/security/probe`) and exposes a stable, general-purpose subscription API so that non-security subsystems (network tracer, GPU monitor, process monitor, USM) can receive kernel-level process lifecycle events — fork, exec, exit — without depending directly on the security module.

The module runs as a `system-probe` module (`module.Module`) and manages the lifecycle of the underlying eBPF probe. Consumers register as `EventConsumer` / `EventConsumerHandler` implementations and receive events through per-consumer channels.

**Build constraint:** `//go:build linux || windows`. A stub file (`eventmonitor_other.go`) satisfies the build on unsupported platforms.

---

## Key elements

### Key types

#### `EventMonitor` (`eventmonitor.go`)

The central struct. Created via `NewEventMonitor` and registered as a `system-probe` module.

```go
type EventMonitor struct {
    Probe        *probe.Probe
    Config       *config.Config
    StatsdClient statsd.ClientInterface
    // internals: context, consumers, stats channel, ...
}
```

Lifecycle:
- `Init()` — loads eBPF programs and maps into the kernel; probes are not yet running.
- `Start()` — snapshots current system state (mounts, running processes), starts all registered `EventConsumer`s, then starts the eBPF probe. Calls `PostProbeStart()` on consumers that implement `EventConsumerPostProbeStartHandler`.
- `Close()` — stops the probe, stops consumers, cancels the context, then closes the probe to unload eBPF programs.

Consumer registration:
- `AddEventConsumerHandler(consumer EventConsumerHandler) error` — registers a handler with the underlying probe. The event type must be in the allowed set: `ForkEventType`, `ExecEventType`, `ExitEventType`, `TracerMemfdSealEventType`.
- `RegisterEventConsumer(consumer EventConsumer)` — registers the consumer's lifecycle (`Start`/`Stop`) with the monitor.

Stats:
- `SendStats()` — synchronously flushes probe statistics to DogStatsD.
- `GetStats() map[string]interface{}` — returns debug statistics including probe state and, if configured, CWS status.

`NewEventMonitor(config, secconfig, hostname, opts) (*EventMonitor, error)` is the constructor. `Opts` carries a `statsd.ClientInterface` and `probe.Opts` (env-var resolution, tagger, workloadmeta).

---

### Key interfaces

#### `EventConsumer` and `EventConsumerHandler` interfaces (`consumer.go`)

Every consumer must implement both interfaces.

**`EventConsumer`** — lifecycle interface:
```go
type EventConsumer interface {
    probe.IDer    // ID() string
    Start() error
    Stop()
}
```

**`EventConsumerHandler`** — event-handling interface (delegates to `probe.EventConsumerHandler`):
```go
// The probe interface requires:
EventTypes() []model.EventType
ChanSize()    int
Copy(ev *model.Event) any   // copy only the needed fields; called on hot path
HandleEvent(ev any)         // called with the value returned by Copy
```

`Copy` runs on the probe's hot path with the event lock held — keep it minimal. `HandleEvent` is called asynchronously from the per-consumer channel goroutine.

**`EventConsumerPostProbeStartHandler`** — optional interface:
```go
type EventConsumerPostProbeStartHandler interface {
    PostProbeStart() error
}
```
Implement this when the consumer needs to perform work after the eBPF probe has started (for example, enrolling existing processes).

---

### Configuration and build flags

Build constraint: `//go:build linux || windows`. A stub file satisfies the build on unsupported platforms. Configuration is loaded from `system-probe.yaml` under the `event_monitoring_config` namespace.

#### `config/` — module configuration

**`Config`** (`config/config.go`)

Minimal config for the event monitoring module itself. Loaded from the `system-probe.yaml` `event_monitoring_config` namespace.

```go
type Config struct {
    EnvVarsResolutionEnabled bool  // event_monitoring_config.env_vars_resolution.enabled
}
```

`NewConfig()` reads from the system-probe configuration.

---

### Key functions

#### `consumers/` — ready-made consumer: `ProcessConsumer`

`ProcessConsumer` (`consumers/process.go`) is a reusable consumer of process exec/exit/fork events. Instead of implementing the full `EventConsumerHandler` interface, callers subscribe to typed callbacks.

**Creating a consumer:**
```go
pc, err := consumers.NewProcessConsumer(
    "my-consumer",         // unique ID
    100,                   // channel buffer size
    []consumers.ProcessConsumerEventTypes{consumers.ExecEventType, consumers.ExitEventType},
    evm,                   // *eventmonitor.EventMonitor
)
```
`NewProcessConsumer` calls `evm.AddEventConsumerHandler` and `evm.RegisterEventConsumer` automatically.

**Subscribing to events:**
```go
unsubExec := pc.SubscribeExec(func(pid uint32) { /* ... */ })
unsubExit := pc.SubscribeExit(func(pid uint32) { /* ... */ })
defer unsubExec()
defer unsubExit()
```
Each subscribe call returns an unsubscribe function. Multiple subscribers are supported concurrently. The internal `callbackMap` uses an `atomic.Bool` fast-path to avoid locking when there are no subscribers.

Supported event type constants: `ExecEventType`, `ExitEventType`, `ForkEventType`.

For testing, use `consumers/testutil.NewTestProcessConsumer` which sets up the event stream without a real system-probe.

---

## Usage

### Module registration (`cmd/system-probe/modules/eventmonitor.go`)

`createEventMonitorModule` is the entry point in system-probe:

1. Create `emconfig.Config` and `secconfig.Config`.
2. Call `eventmonitor.NewEventMonitor(emconfig, secconfig, hostname, opts)`.
3. Optionally create and register the CWS consumer (`secmodule.NewCWSConsumer`).
4. Optionally create and register the network consumer (`events.NewNetworkConsumer`).
5. Optionally create and register additional consumers (USM process monitor, GPU process events, direct sender).
6. Return the `*EventMonitor` — system-probe calls `Register` → `Init` + `Start`.

### Existing consumers in the codebase

| Consumer | Package | Purpose |
|---|---|---|
| CWS (Cloud Workload Security) | `pkg/security/module` | Full security event processing |
| Network | `pkg/network/events` | Tracks process→connection association for NPM |
| Direct sender | `pkg/network/sender` | Sends exec events directly to the backend |
| USM process monitor | `cmd/system-probe/modules` | Feeds the Universal Service Monitor |
| GPU process events | `cmd/system-probe/modules` | Tracks GPU-using processes |

### Writing a custom consumer

1. Implement `EventConsumer` + `EventConsumerHandler` (or use `ProcessConsumer` for process events).
2. In your consumer's `Copy`, copy only the fields you need from `*model.Event` into a small local struct — avoid retaining references to the original event.
3. Register with `evm.AddEventConsumerHandler(yourHandler)` and `evm.RegisterEventConsumer(yourConsumer)`.
4. See `pkg/eventmonitor/examples/consumer.go` for a minimal reference implementation. The `//go:generate` directive there shows how to use the `event_copy` code generator to auto-generate `Copy` from struct tags.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/security/probe` | [`security/probe.md`](security/probe.md) | `EventMonitor` wraps and owns a `*probe.Probe`. All consumer event types (`ForkEventType`, `ExecEventType`, `ExitEventType`, `TracerMemfdSealEventType`) are delivered through the probe's `AddEventConsumer` path. The `CWSConsumer` is itself an `EventConsumer` registered with the event monitor. `EventConsumerHandler.Copy` and `HandleEvent` map directly to `probe.EventConsumerHandler`. |
| `pkg/process/monitor` | [`process/monitor.md`](process/monitor.md) | `pkg/process/monitor` is the higher-level singleton that multiplexes process exec/exit callbacks for USM, Go TLS, Node.js, and Istio monitors. When `useEventStream = true` (i.e. the event monitor is running), `InitializeEventConsumer(consumers.ProcessConsumer)` bridges the eBPF event stream into the `ProcessMonitor` callback dispatch instead of using a separate netlink socket. |
| `pkg/ebpf` | [`ebpf.md`](ebpf.md) | `pkg/ebpf` provides the `Manager`, CO-RE/BTF program loading, `MapCleaner`, and `PerfHandler` infrastructure that `probe.EBPFProbe` (owned by `EventMonitor.Probe`) is built on. The eBPF programs loaded by the probe are managed through `pkg/ebpf.Manager`. |

### Relationship to `pkg/process/monitor`

`pkg/process/monitor` and `pkg/eventmonitor` serve overlapping but distinct roles:

| Aspect | `pkg/process/monitor` | `pkg/eventmonitor` |
|--------|-----------------------|--------------------|
| Event source | Netlink `PROC_EVENT` socket or eBPF event stream | eBPF probe (`pkg/security/probe`) |
| Abstraction level | High-level callback subscription (PID only) | Low-level `model.Event` with full kernel context |
| Consumers | USM, Go TLS, Node.js, Istio uprobe lifecycles | CWS, NPM, USM (via `ProcessConsumer`), GPU |
| Multi-consumer | Yes — singleton with `SubscribeExec`/`SubscribeExit` | Yes — registered `EventConsumer` instances |

When the event monitor is enabled, the two systems are bridged: `consumers.ProcessConsumer`
is created from the event monitor and passed to `monitor.InitializeEventConsumer`, so
both stacks share a single eBPF-sourced event stream.

### Relationship to `pkg/security/probe`

`EventMonitor` exposes the security probe to non-security consumers without requiring
them to import the full `pkg/security` package. The allowed event types are intentionally
limited to `ForkEventType`, `ExecEventType`, `ExitEventType`, and
`TracerMemfdSealEventType` — the probe's `defaultEventTypes` that are always active
regardless of CWS rules. Security-specific event types (file open, network connect, etc.)
are handled exclusively by the `CWSConsumer` and are never exposed through the event
monitor's public consumer API.

### Relationship to `pkg/ebpf`

`EventMonitor.Init()` delegates to `probe.EBPFProbe.Init()`, which calls
`ebpf.LoadCOREAsset` with the security eBPF object file. The resulting eBPF programs and
maps are managed by an `ebpf.Manager` instance inside the probe. The CO-RE / BTF loading
fallback chain (CO-RE → runtime compilation → prebuilt), `MapCleaner`, and
`telemetry.ErrorsTelemetryModifier` are all provided by `pkg/ebpf`. See
[pkg/ebpf](ebpf.md) for the full `Manager` lifecycle and BTF resolution details.

When `EventMonitor.Close()` is called, it calls `probe.Probe.Close()` which calls
`ebpf.Manager.Stop()` to unload all eBPF programs from the kernel. Consumers should
complete their `Stop()` before this point to avoid receiving events from a partially
shut-down probe.
