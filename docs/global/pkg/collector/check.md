# Package `pkg/collector/check`

## Purpose

`pkg/collector/check` defines the **canonical interfaces and shared types** for agent checks. It contains no check implementations; instead, it establishes the contracts that all check implementations (Go core checks, Python checks, JMX checks, shared-library checks) must satisfy, and provides utilities that work against those contracts.

Sub-packages:

| Sub-package | Contents |
|---|---|
| `check/id` | `ID` type and `BuildID` / `IDToCheckName` helpers |
| `check/stats` | `Stats` and `SenderStats` structs for runtime telemetry |
| `check/defaults` | `DefaultCheckInterval` constant (15 s) |
| `check/stub` | Minimal no-op `Check` for testing |

## Key Elements

### `Check` interface

```go
type Check interface {
    Run() error
    Stop()
    Cancel()
    String() string
    Loader() string
    Configure(senderManager sender.SenderManager, integrationConfigDigest uint64,
              config, initConfig integration.Data, source, provider string) error
    Interval() time.Duration
    ID() checkid.ID
    GetWarnings() []error
    GetSenderStats() (stats.SenderStats, error)
    Version() string
    ConfigSource() string
    ConfigProvider() string
    IsTelemetryEnabled() bool
    InitConfig() string
    InstanceConfig() string
    GetDiagnoses() ([]diagnose.Diagnosis, error)
    IsHASupported() bool
}
```

This is the interface every check must implement. Key distinctions:

- `Stop()` is called when the check is actively running and needs to be interrupted.
- `Cancel()` is always called on unschedule, even if the check is not currently running; it may be called concurrently with `Stop()`.
- `Interval() == 0` signals a one-shot check (runs once and is not re-queued).
- `Loader()` must be a lowercase, space-free string because it is used as a tag value.
- `IsHASupported()` controls HA-mode leadership gating in the worker.

### `Info` interface

A read-only subset of `Check` used in contexts that only need metadata (status page, inventory, diagnose). Implemented by `MockInfo` for tests.

```go
type Info interface {
    String() string
    Interval() time.Duration
    ID() checkid.ID
    Version() string
    ConfigSource() string
    ConfigProvider() string
    InitConfig() string
    InstanceConfig() string
}
```

### `Loader` interface

```go
type Loader interface {
    Name() string
    Load(senderManager sender.SenderManager, config integration.Config,
         instance integration.Data, instanceIndex int) (Check, error)
}
```

A `Loader` turns a single YAML instance inside an `integration.Config` into a `Check`. Each loader is registered by priority via `loaders.RegisterLoader` (see `pkg/collector` docs). The scheduler iterates loaders in priority order and stops at the first success.

Return `ErrSkipCheckInstance` when intentionally refusing a config (e.g., a Go check that only handles non-JMX configs, while a Python version handles JMX) — this suppresses noise in the status page.

### `ErrSkipCheckInstance`

```go
var ErrSkipCheckInstance = errors.New("refused to load the check instance")
```

Loaders return this to cleanly decline a config without logging an error. The scheduler only surfaces an error if **all** loaders return non-nil, and only counts `ErrSkipCheckInstance` as a failure if it is the sole result.

### `check/id` — `ID` type and `BuildID`

```go
type ID string

func BuildID(checkName string, integrationConfigDigest uint64,
             instance, initConfig integration.Data) ID
```

`BuildID` hashes the integration config digest, instance YAML, and init-config YAML with FNV-64, producing an ID like `cpu:my_instance:1a2b3c4d` (with the optional instance name when `name:` is set in the instance config).

`IDToCheckName(id)` splits on `:` to recover the check name.

### `check/stats` — `Stats` and `SenderStats`

`Stats` is the per-instance runtime ledger updated by the worker after each run:

| Field | Description |
|---|---|
| `TotalRuns`, `TotalErrors`, `TotalWarnings` | Cumulative counters |
| `ExecutionTimes [32]int64` | Circular buffer of the last 32 run durations (ms) |
| `AverageExecutionTime` | Rolling average of the above buffer |
| `LastError`, `LastWarnings` | Most recent error/warnings |
| `LastSuccessDate` | Unix timestamp of the last clean run |
| `LastDelay` | Gap between scheduled and actual start time |
| `MetricSamples`, `Events`, `ServiceChecks`, etc. | Counts from the most recent run |
| `EventPlatformEvents` | Per event-type counts for DBM, SNMP, etc. |
| `Cancelling` | Set to true while the check is being torn down |

`Stats.Add(duration, err, warnings, senderStats, haAgent)` is the hot path; the worker calls it after every run.

`SenderStats` is a lightweight snapshot of what a `sender.Sender` submitted during one run (metric samples, events, service checks, histogram buckets, event platform events). Checks return it via `GetSenderStats()`.

Telemetry counters emitted by this package (all under the `checks` namespace):

| Metric | Tags | Description |
|---|---|---|
| `checks.runs` | `check_name`, `state` (ok/fail) | Run count |
| `checks.warnings` | `check_name` | Warning count |
| `checks.metrics_samples` | `check_name` | Metric sample count |
| `checks.execution_time` | `check_name`, `check_loader` | Last execution time (ms) |
| `checks.delay` | `check_name` | Check start delay (s) |

### JMX helpers

```go
func IsJMXInstance(name string, instance, initConfig integration.Data) bool
func IsJMXConfig(config integration.Config) bool
func CollectDefaultMetrics(c integration.Config) bool
```

Inspect a config/instance to determine whether it targets a JMX integration. The scheduler uses `IsJMXInstance` to skip JMX instances (they are handled by the JMX loader separately).

### Context and inventory

```go
func InitializeInventoryChecksContext(ic inventorychecks.Component)
func GetInventoryChecksContext() (inventorychecks.Component, error)
func ReleaseContext()
```

A package-level singleton providing access to the `inventorychecks` component for both Go and Python checks. This is a temporary bridge until checks become fx components themselves. Python checks call `SetCheckMetadata` (via CGo) which resolves the component through this context.

### `GetMetadata`

```go
func GetMetadata(c Info, includeConfig bool) map[string]interface{}
```

Builds the metadata map for a check instance used in inventory payloads. When `includeConfig` is true, the instance and init configs are scrubbed of secrets before inclusion.

### `Retry`

```go
func Retry(retryDuration time.Duration, retries int,
           callback func() error, friendlyName string) error
```

Generic retry helper used by check implementations. The callback must return `RetryableError` to request a retry; any other error type aborts immediately. The retry counter resets if the callback ran for at least `retryDuration` before failing.

### `MockInfo`

```go
type MockInfo struct {
    Name, LoaderName string
    CheckID          checkid.ID
    Source, Provider string
    InitConf, InstanceConf string
}
```

Implements `Info`. Use it in tests that need a `check.Info` without a real check instance.

## Usage

### Implementing a Go check

All Go core checks embed `corechecks.CheckBase`, which provides default implementations for the full `Check` interface:

```go
type MyCheck struct {
    corechecks.CheckBase
}

func (c *MyCheck) Run() error {
    sender, err := c.GetSender()
    // collect metrics via sender...
    sender.Commit()
    return err
}

func (c *MyCheck) Configure(senderManager sender.SenderManager, digest uint64,
    instance, initConfig integration.Data, source, provider string) error {
    if err := c.CommonConfigure(senderManager, digest, instance, initConfig, source, provider); err != nil {
        return err
    }
    // parse instance YAML...
    return nil
}
```

`BuildID` must be called from `Configure` if the check supports multiple instances so that each instance gets a unique `ID`.

### Implementing a `Loader`

```go
type myLoader struct{}

func (l *myLoader) Name() string { return "myloader" }

func (l *myLoader) Load(sm sender.SenderManager, cfg integration.Config,
    instance integration.Data, idx int) (check.Check, error) {
    c := &MyCheck{CheckBase: corechecks.NewCheckBase("my_check")}
    if err := c.Configure(sm, cfg.FastDigest(), instance, cfg.InitConfig, cfg.Source, cfg.Provider); err != nil {
        return nil, err
    }
    return c, nil
}

func init() {
    loaders.RegisterLoader(func(sm sender.SenderManager, ...) (check.Loader, int, error) {
        return &myLoader{}, 10, nil // 10 = priority
    })
}
```

The loader's `Name()` return value is used both for logging and as the `loader:` config key that allows users to pin a specific loader in `datadog.yaml`.

### Working with check IDs

```go
id := checkid.BuildID("cpu", cfg.FastDigest(), instance, initConfig)
// id is something like "cpu:1a2b3c4d" or "cpu:my_instance:1a2b3c4d"

name := checkid.IDToCheckName(id) // "cpu"
```

### Using `Retry` in a check

```go
func (c *MyCheck) Run() error {
    return check.Retry(5*time.Second, 3, func() error {
        if err := c.connect(); err != nil {
            return check.RetryableError{Err: err}
        }
        return c.collect()
    }, "my_check.collect")
}
```

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/collector/corechecks`](corechecks.md) | Provides `CheckBase`, the embeddable base struct used by all Go check implementations. The `GoCheckLoader` in that package produces `Check` values from this interface. |
| [`pkg/collector/python`](python.md) | `PythonCheck` (loaded via `PythonCheckLoader`) implements this `Check` interface. The Python loader has priority 20; Go core checks have priority 30. |
| [`pkg/collector/runner`](runner.md) | Consumes `Check` values from a channel, dispatches them to worker goroutines, and calls `check.Run()`. |
| [`pkg/collector/worker`](worker.md) | The per-goroutine executor that calls `check.Run()`, records `check/stats.Stats`, and emits service checks via the sender. |
| [`pkg/collector/loaders`](loaders.md) | Registry of `check.Loader` factories ordered by priority. The `CheckScheduler` walks this list to find the first loader that accepts a given integration config. |
| [`pkg/collector` (CheckScheduler)](collector.md) | Calls `Loader.Load()` for each resolved integration config and forwards resulting `Check` instances to the collector component. |
| [`pkg/aggregator/sender`](../aggregator/sender.md) | `Sender` / `SenderManager` are passed to `Configure()` so checks can submit metrics. `SenderStats` (mirrored in `check/stats`) tracks what a `Sender` submitted during one run. |
| [`pkg/telemetry`](../telemetry.md) | `check/stats` emits per-check counters (`checks.runs`, `checks.execution_time`, etc.) through `pkg/telemetry`. |
