> **TL;DR:** `pkg/security/probe` owns the cross-platform kernel event probe — capturing security events via eBPF or ETW, decoding them into SECL `model.Event` values, and dispatching them to the rule engine and registered consumers.

# pkg/security/probe — Kernel-space event probe

## Purpose

`pkg/security/probe` owns the runtime security **probe**: the component that programs the kernel (via eBPF on Linux, ETW/kernel-file callbacks on Windows) to capture security-relevant events, decodes them into the SECL `model.Event` type, and dispatches them to registered consumers and rule handlers.

The package is intentionally cross-platform: `probe.go` and `eventconsumer.go` are platform-agnostic; platform-specific logic lives in `probe_linux.go`, `probe_ebpf.go`, `probe_windows.go`, etc., all behind a `PlatformProbe` interface.

## Key elements

### Key types

| Type | File | Description |
|------|------|-------------|
| `Probe` | `probe.go` | Top-level probe struct. Holds configuration, consumer/handler registries, rule-action statistics, and delegates all platform work to `PlatformProbe`. Build constraint: `linux \|\| windows`. |
| `EBPFProbe` | `probe_ebpf.go` | Linux eBPF implementation of `PlatformProbe`. Manages the `ebpf-manager` Manager, kernel constants, discarders, approvers (kfilters), activity dumps, on-demand probes, process killing, and file hashing. Linux only. |
| `EBPFLessProbe` | `probe_ebpfless.go` | Alternative Linux implementation that collects events from an external ptrace-based tracer rather than eBPF. Useful for environments where eBPF is unavailable. |
| `EBPFMonitors` | `probe_monitor.go` | Aggregates all monitoring sub-systems: `eventstream.Monitor`, `discarder.Monitor`, `approver.Monitor`, `syscalls.Monitor`, `cgroups.Monitor`, `dns.Monitor`, `eventsample.Monitor`. Linux only. |
| `EventConsumer` | `probe.go` | Wraps an `EventConsumerHandler` with a buffered channel and a dropped-event counter. Started as a goroutine by `Probe.Start`. |
| `Discarder` | `discarders.go` | Represents a kernel-space discarder: a field whose value is known to never match any active rule, so the kernel can drop the event before sending it to user space. |
| `ProcessKiller` | `process_killer.go` | Executes kill actions triggered by rules (sends SIGKILL to a process). |
| `FileHasher` | `file_hasher.go` | Computes file hashes asynchronously as part of hash-action rule responses. |
| `OnDemandProbesManager` | `on_demand.go` | Dynamically attaches/detaches additional eBPF probes at runtime without requiring a full rule reload. Capped at `MaxOnDemandEventsPerSecond = 1000`. |

### Key interfaces

| Interface | File | Description |
|-----------|------|-------------|
| `PlatformProbe` | `probe.go` | Implemented by `EBPFProbe`, `EBPFLessProbe`, and the Windows probe. Defines the full lifecycle (`Init`, `Start`, `Stop`, `Close`), rule-set application (`ApplyRuleSet`, `OnNewRuleSetLoaded`), discarder management, and event replay. |
| `EventStream` | `probe_ebpf.go` | Abstraction over perf-buffer (`reorderer`) and ring-buffer transport layers. Implemented by `reorderer.ReOrderMonitor` and `ringbuffer.RingBuffer`. Linux only. |
| `EventHandler` | `eventconsumer.go` | `HandleEvent(*model.Event)` — receives fully-decoded events with all struct fields accessible. Registered via `Probe.AddEventHandler`. |
| `EventConsumerHandler` | `eventconsumer.go` | `Copy(*model.Event) any` + `HandleEvent(any)` — receives a copy of the event on a buffered channel, isolating the consumer from the hot path. Registered via `Probe.AddEventConsumer`. |
| `CustomEventHandler` | `eventconsumer.go` | `HandleCustomEvent(*rules.Rule, *events.CustomEvent)` — receives synthetic events. Registered via `Probe.AddCustomEventHandler`. |
| `DiscarderPushedCallback` | `probe.go` | `func(eventType string, event *model.Event, field string)` — called when a discarder is pushed to the kernel map. |
| `LostEventCounter` | `eventstream/monitor.go` | `CountLostEvent(count uint64, perfMapName string, CPU int)` — implemented by `eventstream.Monitor`. |

### Sub-packages

#### `constantfetch/`

Resolves kernel struct offsets and sizes needed for eBPF CO-RE relocations.

| Type / function | Description |
|-----------------|-------------|
| `ConstantFetcher` | Interface: `AppendSizeofRequest`, `AppendOffsetofRequestWithFallbacks`, `FinishAndGetResults`. |
| `ComposeConstantFetcher` | Chains multiple fetchers with fallback semantics; caches results by MD5 of request list. |
| `BTFConstantFetcher` | Reads offsets from BTF data (current kernel or a BTF file). Build tag: `linux && linux_bpf`. |
| `FallbackConstantFetcher` | Hand-crafted offset table for kernels without BTF. |
| `OffsetGuesser` | Empirically guesses offsets at runtime (last resort). |
| `ErrorSentinel` | `^uint64(0)` — returned for any offset that could not be resolved. |
| `CreateConstantEditors` | Converts a `ConstantFetcherStatus` into `[]manager.ConstantEditor` for the ebpf-manager. |

Fetchers are composed in priority order: BTF CO-RE > BTFHub archive > fallback table > offset guesser.

#### `eventstream/`

Manages the perf-buffer / ring-buffer transport from kernel to user space.

| Type | Description |
|------|-------------|
| `Monitor` | Tracks per-CPU, per-event-type byte and event counts (both kernel-side and user-side). Detects lost events and notifies via `onEventLost` callback. Reports stats via statsd. |
| `EventStreamMap` | Constant `"events"` — the eBPF map name for the main event stream. |
| `reorderer/` | Reorders out-of-order perf-buffer events using a timestamp-based priority queue. |
| `ringbuffer/` | Wraps the cilium/ebpf ring-buffer for kernels >= 5.8. |

#### `kfilters/`

Translates SECL rule approvers into eBPF map entries that filter events in-kernel.

| Type / symbol | Description |
|---------------|-------------|
| `KFilterGetters` | `map[eval.EventType]kfiltersGetter` — one getter per supported event type (open, chmod, chown, rename, unlink, mkdir, rmdir, link, utimes, mmap, mprotect, splice, chdir, bpf, sysctl, connect, prctl, setsockopt). |
| `Capababilities` | `map[eval.EventType]rules.FieldCapabilities` — describes which rule fields can be pushed down to the kernel (basename, path, flags, auid, in-upper-layer). |
| `GetCapababilities()` | Returns all supported kernel-side filtering capabilities. Used by `RuleEngine` when building approvers. |
| `FilterPolicy` | Policy mode (`accept` / `deny`) serialized into the kernel approver map. |
| `BasenameApproverKernelMapName` | `"basename_approvers"` |
| `InUpperLayerApproverKernelMapName` | `"in_upper_layer_approvers"` |

#### `monitors/`

Per-subsystem eBPF map monitors, all reporting via statsd:

| Sub-package | Monitors |
|-------------|----------|
| `approver/` | Approver hit/miss counters. |
| `discarder/` | Inode and pid discarder counters; ring buffer usage. |
| `cgroups/` | Container/cgroup lifecycle events. |
| `dns/` | DNS resolution request counts. |
| `syscalls/` | Per-syscall event counts (optional; enabled by `Opts.SyscallsMonitorEnabled`). |
| `eventsample/` | Sample of raw events for debugging. |

#### Other notable sub-packages

| Sub-package | Purpose |
|-------------|---------|
| `config/` | `ProbeConfig` struct — low-level probe tunables (ring buffer size, network enabling, stats polling interval, etc.). |
| `erpc/` | User-space ↔ kernel communication channel used for discarder invalidation and ring-buffer usage queries. |
| `managerhelper/` | Helpers for building `ebpf-manager` options (constant editors, perf maps, probes). |
| `selftests/` | Probe self-test harness: runs controlled operations and verifies detection. |
| `sysctl/` | Monitor for sysctl changes via cgroup BPF programs. |
| `procfs/` | Fallback process information reader from `/proc`. |

### Key functions and constants

| Symbol | Value / Description |
|--------|---------------------|
| `EBPFOrigin` | `"ebpf"` |
| `EBPFLessOrigin` | `"ebpfless"` |
| `MaxOnDemandEventsPerSecond` | `1_000` — threshold above which on-demand probes are disabled. |
| `SupportedDiscarders` | `map[eval.Field]bool` — set of fields that support inode/pid discarders; populated by `init()` in `discarders_linux.go`. |
| `SupportedMultiDiscarder` | `[]*rules.MultiDiscarder` — compound discarders (e.g. process + file path). |
| `ErrorSentinel` (`constantfetch`) | `^uint64(0)` — sentinel for an unresolved kernel constant. |
| `defaultEventTypes` | `[fork, exec, exit, tracer_memfd_seal]` — always-on event types regardless of active rules. |

### Configuration and build flags

| Flag | Effect |
|------|--------|
| `linux` | eBPF probe and all Linux-only code paths. |
| `linux_bpf` | BTF CO-RE `BTFConstantFetcher` and ring-buffer support. |
| `windows` | Windows ETW / kernel-file / registry probes. |

## Usage

### Instantiating a probe

```go
cfg, _ := config.NewConfig()
opts := probe.Opts{
    StatsdClient:          statsdClient,
    PathResolutionEnabled: true,
    SyscallsMonitorEnabled: true,
}
p, err := probe.NewProbe(cfg, hostname, opts)
```

On Linux, `NewProbe` picks `EBPFProbe` or `EBPFLessProbe` based on `opts.EBPFLessEnabled`.

### Registering consumers

```go
// Full-event access (handler runs in-line in the hot path):
p.AddEventHandler(myHandler)

// Async consumer (receives a copy on a buffered channel):
p.AddEventConsumer(myConsumer) // myConsumer implements EventConsumerHandler

// Synthetic events:
p.AddCustomEventHandler(model.UnknownEventType, myCustomHandler)
```

### Rule set lifecycle

```go
rs := p.NewRuleSet(enabledEventTypes)
// ... populate rs with rules ...
filterReport, ruleSetChanged, err := p.ApplyRuleSet(rs)
p.OnNewRuleSetLoaded(rs)
```

`ApplyRuleSet` computes kfilter approvers from the rule set, writes them to eBPF maps, and returns a `kfilters.FilterReport` describing which fields were pushed to the kernel.

### Discarder flow

When the rule evaluator determines that no active rule can match a particular inode or pid, it calls `Probe.OnNewDiscarder`. The platform probe writes the discarder into the kernel eBPF map (via `erpc` or direct map write), so the kernel silently drops future events for that inode/pid without context-switching to user space.

### Kernel constant resolution

During `EBPFProbe.Init()`, `constantfetch.ComposeConstantFetchers` is called with fetchers in priority order. The resolved offsets are converted to `manager.ConstantEditor` entries and injected into the eBPF program before it is loaded. If `ErrorSentinel` is returned for a required field, the offset defaults to `0` and a log error is emitted.

### eBPF-less mode (ptracer)

When `opts.EBPFLessEnabled = true`, `NewProbe` instantiates `EBPFLessProbe` instead of `EBPFProbe`. In this mode, no eBPF programs are loaded. Instead, `system-probe` opens a TCP listener and waits for `cws-instrumentation` (the `pkg/security/ptracer` binary) to connect. The ptracer intercepts syscalls via `ptrace(2)`, converts them to `ebpfless.SyscallMsg` protobuf messages, and streams them to `EBPFLessProbe`, which decodes and dispatches them through the same `Probe.DispatchEvent` path as eBPF events.

From the perspective of `RuleEngine` and the rest of CWS, events from both probe types are identical `model.Event` values. The probe origin is recorded in `event.Origin` (`"ebpf"` vs `"ebpfless"`), which can be used in SECL rule `filters:` via the `origin` field.

See [ptracer.md](ptracer.md) for the `cws-instrumentation` side of this integration.

### Importers of this package

The main importers are:

- `pkg/security/rules` (`RuleEngine`) — calls `Probe.ApplyRuleSet`, `NewRuleSet`, `DispatchEvent`.
- `pkg/security/module` (`CWSConsumer`, `APIServer`) — creates the probe, registers as custom-event handler, calls dump and status APIs.
- `pkg/security/tests` — functional tests that instantiate the probe directly against a live kernel.

---

## Related documentation

| Doc | Description |
|-----|-------------|
| [security.md](security.md) | Top-level CWS integration hub: shows how `Probe` is owned by `CWSConsumer` and how the full event pipeline is wired together. |
| [secl.md](secl.md) | SECL language overview: `RuleSet.Evaluate(event)` and `GetApprovers()` that `ApplyRuleSet` depends on. |
| [secl-model.md](secl-model.md) | `model.Event` struct filled by `EBPFProbe`; `FieldHandlers` interface implemented by `field_handlers_ebpf.go`. |
| [resolvers.md](resolvers.md) | `EBPFResolvers` created inside `probe_ebpf.go`; individual resolvers are invoked from `FieldHandlers` to lazily populate event fields. |
| [rules.md](rules.md) | `RuleEngine.HandleEvent` is registered as an `EventHandler` on the probe; `EventDiscarderFound` feedback loop drives `OnNewDiscarder`. |
| [../../pkg/ebpf.md](../../pkg/ebpf.md) | Shared eBPF infrastructure: `Manager` lifecycle, CO-RE asset loading, `MapCleaner`, perf/ring-buffer handlers — all consumed by `EBPFProbe`. |
