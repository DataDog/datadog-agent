# pkg/collector/aggregator

**Import path:** `github.com/DataDog/datadog-agent/pkg/collector/aggregator`

## Purpose

This package is the Cgo bridge between the Python/rtloader runtime and the Go aggregator. It exposes C-callable functions (via `//export` directives) that Python checks call to submit metrics, service checks, events, histogram buckets, and event-platform events. It also manages the global `CheckContext`, which holds the references all submission functions need (sender manager, tagger, log receiver, workload filter).

This package requires Cgo. It is used only when the agent is built with Python check support (the `python` build tag, implicitly required when linking against rtloader).

## Key Elements

### CheckContext (`check_context.go`)

The singleton that wires together the dependencies required by every submission function.

| Field | Type | Purpose |
|---|---|---|
| `senderManager` | `sender.SenderManager` | Looks up a `Sender` by check ID |
| `logReceiver` | `option.Option[integrations.Component]` | Optional log receiver for integration-emitted logs |
| `tagger` | `tagger.Component` | Resolves entity tags |
| `filter` | `workloadfilter.FilterBundle` | Container-level metric filtering |

**`InitializeCheckContext(...)`** — must be called exactly once during agent startup (see `pkg/collector/python/loader.go`). Subsequent calls are silently ignored (the guard is a mutex + nil-check).

**`GetCheckContext() (*CheckContext, error)`** — called by every exported C function to retrieve the singleton; returns an error if `InitializeCheckContext` was never called.

### Exported C functions (`aggregator.go`)

All functions are exported with `//export` so they can be called from C/rtloader:

| Function | Description |
|---|---|
| `SubmitMetric` | Dispatches gauge, rate, count, monotonic count, counter, histogram, or historate to the sender |
| `SubmitServiceCheck` | Submits a service check status |
| `SubmitEvent` | Submits a structured event |
| `SubmitHistogramBucket` | Submits a pre-computed histogram bucket |
| `SubmitEventPlatformEvent` | Submits a raw event-platform payload (e.g., DBM, network events) |

All functions resolve the sender by check ID via `senderManager.GetSender(checkid.ID(...))` before forwarding.

### CString helpers (`helpers.go`)

**`CStringArrayToSlice(unsafe.Pointer) []string`** — converts a null-terminated `**C.char` array to a `[]string`. Uses a shared `stringInterner` (capped at 1000 entries, `inter.go`) to reduce allocations for repeated tag strings. Avoids calling `C.strlen` to save CGo call overhead.

**`CStringArrayToSlice`** is also exported for use by other packages in `pkg/collector/python` (tagger, containers).

### String interner (`inter.go`)

`stringInterner` is a simple map-based string intern table, shared across calls via `acquireInterner()`. The shared interner is reset when it exceeds `maxInternerStrings = 1000` entries to prevent unbounded memory growth.

## Usage

1. **Initialization** — `pkg/collector/python/loader.go` calls `InitializeCheckContext` once, passing the component-provided sender manager, tagger, log receiver, and workload filter.
2. **Runtime** — When a Python check calls e.g. `datadog_agent.submit_metric(...)`, rtloader forwards the call through the C ABI to `SubmitMetric`, which retrieves the `CheckContext`, looks up the sender for that check ID, and calls the appropriate `Sender` method.
3. **Tag conversion** — All C tag arrays are converted to Go slices via `CStringArrayToSlice` before being passed to the sender; the interner ensures tags shared across many checks (e.g. `env:prod`) are stored as a single string instance.

The package has no public interface beyond `InitializeCheckContext`, `GetCheckContext`, and `CStringArrayToSlice` — the exported C symbols are consumed solely by rtloader.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/collector/python`](python.md) | `pkg/collector/python/loader.go` calls `InitializeCheckContext` exactly once at agent startup, injecting the sender manager, tagger, log receiver, and workload filter. The `aggregator` callback module that Python checks invoke (via `self.gauge(...)`, `self.event(...)`, etc.) resolves to the exported C functions in this package. |
| [`pkg/aggregator`](../aggregator/aggregator.md) | This package is the Cgo bridge *into* the aggregator. Downstream, `senderManager.GetSender(checkid.ID)` returns a `sender.Sender` backed by the `BufferedAggregator`'s `CheckSampler`. Each call to `SubmitMetric` (and analogues) ultimately queues a sample into that `CheckSampler`. The aggregator doc covers the full flush path from `CheckSampler` to the serializer. |
| [`pkg/tagger`](../tagger.md) | The `CheckContext` holds a `tagger.Component` reference. Although the aggregator bridge itself does not call the tagger directly, the `filter` field (a `workloadfilter.FilterBundle`) uses tagger-resolved container metadata to gate which container-level metrics are forwarded. The tagger component is wired in via `InitializeCheckContext`. |
| [`pkg/collector/sharedlibrary`](sharedlibrary.md) | The same five submission symbols (`SubmitMetric`, etc.) are exposed to native shared-library checks via `ffi.h`'s `aggregator_t` callback struct. Both the Python path (through rtloader) and the shared-library path ultimately call the same exported Go functions from this package. |
