# pkg/process/runner

> **TL;DR:** `pkg/process/runner` implements the check execution and real-time scheduling loop for the process-agent, handing completed payloads to `CheckSubmitter` which encodes, queues, and forwards them to the Datadog intake.

**Import path:** `github.com/DataDog/datadog-agent/pkg/process/runner`

## Purpose

Implements the check execution loop for the process-agent (and the process component of the core agent). `CheckRunner` schedules each enabled process check on its own goroutine, supports a real-time (RT) mode with a shorter collection interval, and hands completed payloads off to a `Submitter` for forwarding to the Datadog intake.

`CheckSubmitter` (in `submitter.go`) is the production `Submitter`. It encodes payloads, queues them into per-check-type `WeightedQueue`s, forwards them via the forwarder component, parses backend responses, and signals the runner to enable or disable RT mode.

## Key Elements

### Key types

#### CheckRunner (`runner.go`)

| Field | Type | Purpose |
|---|---|---|
| `enabledChecks` | `[]checks.Check` | Checks to run; set at construction |
| `Submitter` | `Submitter` | Receives encoded payloads after each run |
| `realTimeEnabled` | `*atomic.Bool` | Whether RT checks are currently active |
| `realTimeInterval` | `time.Duration` | Live-adjustable RT collection interval (default 2 s) |
| `rtNotifierChan` | `<-chan types.RTResponse` | Receives RT on/off signals from the submitter |
| `runRealTime` | `bool` | Static config flag; if false, RT mode is never enabled |

**`NewRunner(...)`** — preferred constructor; reads `process_config.disable_realtime_checks` from config and wires system-probe settings.

**`NewRunnerWithChecks(...)`** — lower-level constructor used in tests or when the caller manages RT and sysprobe settings directly.

**`Run() error`** — initializes each check, selects a runner goroutine strategy, and starts one goroutine per check. Also starts `listenForRTUpdates` if RT is allowed.

**`Stop()`** — closes the `stop` channel, waits for all goroutines, then calls `Cleanup()` on each check.

**`UpdateRTStatus([]*model.CollectorStatus)`** — called from the RT notifier goroutine; enables or disables real-time mode and broadcasts an interval update to all check goroutines via `rtIntervalCh`.

#### Runner strategies

`runnerForCheck(c)` returns one of two goroutine functions:

- **`basicRunner`** — for checks that do not support `RunOptions` or when RT is globally disabled. Ticks on the standard check interval; real-time checks run only when `realTimeEnabled` is true.
- **`checks.NewRunnerWithRealTime`** — for checks that implement `SupportsRunOptions()`. Runs standard and RT collections on the same goroutine, interleaved based on the ratio of their intervals. Interval overrides that violate the divisibility constraint are silently reset to defaults with a warning log.

### Key interfaces

#### Submitter interface and CheckSubmitter (`submitter.go`)

```go
type Submitter interface {
    Submit(start time.Time, name string, messages *types.Payload)
}
```

`CheckSubmitter` is the concrete implementation:

| Concern | Detail |
|---|---|
| Queues | Three `api.WeightedQueue`s: process/discovery/containers, RT process/RT containers, connections. Bounded by `process_config.queue_size` (item count) and `process_config.process_queue_bytes` (byte weight) |
| Forwarding | Per-check-type `submitFunc` maps to the matching forwarder endpoint (e.g. `SubmitProcessChecks`, `SubmitRTProcessChecks`, `SubmitConnectionChecks`) |
| Headers | Each payload gets timestamp, hostname, agent version, container count, content-type, agent start time, payload source, and request-ID headers |
| Request ID | 64-bit integer: 22 bits of seconds-in-month + 28 bits FNV hash of hostname+PID + 14 bits chunk index. Cached after first computation |
| RT signaling | Parses `model.ResCollector` responses and sends `CollectorStatus` slices to `rtNotifierChan`; the runner calls `UpdateRTStatus` from a separate goroutine |
| Heartbeat | When running as `process-agent` flavor, emits `datadog.process.agent` gauge via DogStatsD every 15 s |
| Drop list | Payloads for checks listed in `process_config.drop_check_payloads` are silently discarded before forwarding |

**`NewSubmitter(...)`** — the constructor; resolves API endpoints via `endpoint.GetAPIEndpoints`, builds the queue map and submit-function map.

### Key functions

#### endpoint sub-package (`runner/endpoint`)

**`GetAPIEndpoints(config) ([]apicfg.Endpoint, error)`** — reads `process_config.process_dd_url` (primary) and `process_config.additional_endpoints` (multi-endpoint) from config to build the list of intake endpoints.

### Configuration and build flags

Configuration keys for queue sizes, endpoint URLs, RT-mode, and payload dropping are listed in the `### Configuration keys` table in the `## Usage` section.

## Usage

`pkg/process/runner` is consumed by the fx component `comp/process/runner/runnerimpl`:

1. `runnerimpl.newRunner` calls `processRunner.NewRunner(...)` with the checks provided by the `check` group, host info, and the RT notifier channel sourced from `CheckSubmitter.GetRTNotifierChan()`.
2. The fx lifecycle hooks call `runner.Run()` on start and `runner.Stop()` on stop.
3. `CheckSubmitter.Start()` is called separately by `comp/process/submitter/submitterimpl`.

For the standalone process-agent binary (`cmd/process-agent`), the same wiring applies but via the `ProcessAgent` flavor; the heartbeat goroutine is only active in that flavor.

### Configuration keys

| Key | Default | Description |
|---|---|---|
| `process_config.disable_realtime_checks` | `false` | Globally disable RT mode |
| `process_config.queue_size` | `DefaultProcessQueueSize` | Max items per weighted queue |
| `process_config.process_queue_bytes` | `DefaultProcessQueueBytes` | Max byte weight per queue |
| `process_config.rt_queue_size` | `DefaultProcessRTQueueSize` | Max items in the RT queue |
| `process_config.drop_check_payloads` | `[]` | Check names whose payloads are dropped |
| `process_config.process_dd_url` | (derived from `api_key`) | Primary intake endpoint URL |
| `process_config.additional_endpoints` | `{}` | Extra endpoint URL → API key map |
