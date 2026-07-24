# Check collector

-----

The check collector is the subsystem that turns integration configurations into running *checks*: units of collection code invoked periodically that push metrics, events, and service checks into the aggregator through a per-check `Sender`. It is split between a thin Fx component, [`comp/collector/collector`](<<<SRC>>>/comp/collector/collector), which owns the runtime (interval scheduler, worker pool, and the set of live check instances), and the legacy [`pkg/collector`](<<<SRC>>>/pkg/collector) tree, which contains the mechanics: the Autodiscovery-facing `CheckScheduler`, the catalog of check *loaders*, a time-bucketed interval scheduler, and a runner with a dynamic pool of worker goroutines that actually call `Check.Run()`.

This page covers the pipeline itself. The check implementations have their own pages: [Go core checks](corechecks.md), [Python checks and rtloader](python.md), and [JMX checks](jmx.md) (which bypass this pipeline entirely). The origin of all configurations is [Autodiscovery](autodiscovery.md).

## Topology

```text
 conf.d files, container annotations,      ┌─────────────────────┐
 kube services, cluster-check dispatch, ──▶│    Autodiscovery    │
 remote config, ...                        └──────────┬──────────┘
                                                      │ Schedule([]integration.Config)
                                                      ▼
                                           ┌─────────────────────┐   loaders: core → python → sharedlibrary
                                           │   CheckScheduler    │──▶ Loader.Load() → check.Check
                                           │  (pkg/collector)    │
                                           └──────────┬──────────┘
                                                      │ collector.RunCheck(check)
                                                      ▼
                                           ┌─────────────────────┐
                                           │ collector component │   wraps in CheckWrapper,
                                           │ (comp/collector)    │   sizes the worker pool
                                           └──────────┬──────────┘
                                                      │ scheduler.Enter(check)
                                                      ▼
                                           ┌─────────────────────┐
                                           │ scheduler.Scheduler │   one jobQueue per distinct interval,
                                           │ (interval ticker)   │   1-second buckets
                                           └──────────┬──────────┘
                                                      │ checksPipe (chan check.Check)
                                                      ▼
                                           ┌─────────────────────┐
                                           │ runner.Runner       │   N worker goroutines
                                           │ + worker.Worker     │   call check.Run()
                                           └──────────┬──────────┘
                                                      │ sender.Sender (per check ID)
                                                      ▼
                                              aggregator / demultiplexer
```

/// warning | Two different "schedulers"
The codebase contains two types with confusingly similar names. [`pkg/collector.CheckScheduler`](<<<SRC>>>/pkg/collector/scheduler.go) is the Autodiscovery bridge: it receives `integration.Config` objects and loads them into check instances. [`pkg/collector/scheduler.Scheduler`](<<<SRC>>>/pkg/collector/scheduler/scheduler.go) is the interval ticker that decides *when* an already-loaded check runs. This page calls the former the "check scheduler" and the latter the "collector scheduler" or "job queues".
///

## Key packages

| Path | Purpose |
|---|---|
| [`comp/collector/collector/def/component.go`](<<<SRC>>>/comp/collector/collector/def/component.go) | Component interface: `RunCheck`, `StopCheck`, `GetChecks`, `MapOverChecks`, `ReloadAllCheckInstances`, `AddEventReceiver` |
| [`comp/collector/collector/impl/collector.go`](<<<SRC>>>/comp/collector/collector/impl/collector.go) | Implementation: owns the scheduler and runner, map of running checks, lifecycle hooks, Python init |
| [`comp/collector/collector/impl/internal/middleware/check_wrapper.go`](<<<SRC>>>/comp/collector/collector/impl/internal/middleware/check_wrapper.go) | `CheckWrapper` middleware around every running check |
| [`comp/collector/collector/noop-impl`](<<<SRC>>>/comp/collector/collector/noop-impl) | No-op implementation for binaries without a collector |
| [`pkg/collector/scheduler.go`](<<<SRC>>>/pkg/collector/scheduler.go) | `CheckScheduler`: the Autodiscovery scheduler named `check` |
| [`pkg/collector/check/check.go`](<<<SRC>>>/pkg/collector/check/check.go) | The `Check` interface, `ErrSkipCheckInstance`, `IssueAwareCheck` |
| [`pkg/collector/check/id/id.go`](<<<SRC>>>/pkg/collector/check/id/id.go) | Check ID construction (`name:hexhash`) |
| [`pkg/collector/loaders/loaders.go`](<<<SRC>>>/pkg/collector/loaders/loaders.go) | Loader factory catalog with priorities |
| [`pkg/collector/scheduler`](<<<SRC>>>/pkg/collector/scheduler) | Interval scheduler: job queues, one-second buckets |
| [`pkg/collector/runner/runner.go`](<<<SRC>>>/pkg/collector/runner/runner.go) | Runner: worker pool sizing and stop semantics |
| [`pkg/collector/worker/worker.go`](<<<SRC>>>/pkg/collector/worker/worker.go) | Worker loop: HA gating, dedup, panic recovery, stats |
| [`pkg/collector/runner/expvars`](<<<SRC>>>/pkg/collector/runner/expvars) | Central expvar store for check stats (feeds `agent status`) |
| [`pkg/collector/infra_mode.go`](<<<SRC>>>/pkg/collector/infra_mode.go) | `IsCheckAllowed`: infrastructure-mode allow/exclude gating |
| [`pkg/collector/stats.go`](<<<SRC>>>/pkg/collector/stats.go) | Loader/run error bookkeeping (expvar `CheckScheduler`) |
| [`pkg/collector/metriclookback`](<<<SRC>>>/pkg/collector/metriclookback) | Shadow-check (metric lookback) selection policy |

## The Check interface

Every check implements [`check.Check`](<<<SRC>>>/pkg/collector/check/check.go): `Run`, `Stop`, `Cancel`, `String`, `Loader`, `Configure`, `Interval`, `ID`, `GetWarnings`, `GetSenderStats`, `Version`, `ConfigSource`, `ConfigProvider`, `IsTelemetryEnabled`, `InitConfig`, `InstanceConfig`, `GetDiagnoses`, and `IsHASupported`. The key lifecycle semantics:

1. `Configure(senderManager, configDigest, instance, initConfig, source, provider)` is called exactly once, by the loader, before the check is handed to the collector.
1. `Run()` performs one collection cycle and returns. A check with `Interval() == 0` is treated as one-shot or long-running: it is scheduled once, immediately, and occupies a worker until `Run` returns.
1. `Stop()` is called while the check is running and should interrupt `Run` (relevant mostly for long-running checks).
1. `Cancel()` is always called when the check is unscheduled, whether or not it is running, and frees resources (for Python checks it releases interpreter references).

`Configure` may return the sentinel [`check.ErrSkipCheckInstance`](<<<SRC>>>/pkg/collector/check/check.go) to mean "this configuration is not for my implementation, try another loader". It is not logged as an error unless *every* loader fails; this is the mechanism that lets a Go and a Python implementation of the same integration coexist (see [Go core checks](corechecks.md)).

Check IDs ([`checkid.BuildID`](<<<SRC>>>/pkg/collector/check/id/id.go)) have the form `<name>:<fnv64-hex>`, where the hash covers the config digest, instance, and `init_config`. If the instance sets a `name:` field, it is embedded for readability: `<name>:<instance-name>:<fnv64-hex>`. Checks with a single implicit instance (most core system checks) use the bare check name as their ID. Because the ID includes the config digest, editing a configuration produces a *different* check ID: the old instance is stopped and a new one is started.

## CheckScheduler: from config to check instance

[`CheckScheduler`](<<<SRC>>>/pkg/collector/scheduler.go) is registered with Autodiscovery under the scheduler name `check` by each binary that runs checks: the core agent in [`cmd/agent/subcommands/run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go), the Cluster Agent (DCA) in [`cmd/cluster-agent/subcommands/start/command.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/start/command.go), and the Cloud Foundry cluster agent in [`cmd/cluster-agent-cloudfoundry/subcommands/run/command.go`](<<<SRC>>>/cmd/cluster-agent-cloudfoundry/subcommands/run/command.go). The `agent check` CLI creates one but never attaches it to Autodiscovery (one-shot use).

`CheckScheduler.Schedule(configs)` does the following for each config:

1. Skips configs that are not check configs (for example logs-only configs) and configs excluded for metrics collection by the workload filter (`config.HasFilter(workloadfilter.MetricsFilter)`).
1. Skips JMX instances (`check.IsJMXInstance`) — those are consumed by the separate `jmx` Autodiscovery scheduler, see [JMX checks](jmx.md).
1. For each remaining instance, walks the loader catalog in priority order until one loader returns a check. A config can pin a loader with a `loader:` key in `init_config` or in the instance (the instance wins). Loader errors are only reported to the status page when *all* loaders fail.
1. Records `configDigest → []checkID` in `configToChecks` (used later for unscheduling).
1. Applies infrastructure-mode gating (`IsCheckAllowed`, see below) and calls `collector.RunCheck(check)`.

### Loaders

The loader catalog ([`pkg/collector/loaders`](<<<SRC>>>/pkg/collector/loaders)) is populated by `init()` functions; each loader factory registers with an integer priority and the catalog is sorted ascending (lowest priority value tried first):

| Loader | Name | Priority | Build tag / gate | Source |
|---|---|---|---|---|
| Go core check loader | `core` | 10 if `prioritize_go_check_loader` (default `true`), else 30 | always built | [`pkg/collector/corechecks/loader.go`](<<<SRC>>>/pkg/collector/corechecks/loader.go) |
| Python loader | `python` | 20 | `python` build tag | [`pkg/collector/python/loader.go`](<<<SRC>>>/pkg/collector/python/loader.go) |
| Shared-library (Rust) loader | `sharedlibrary` | 40 | `sharedlibrarycheck` build tag + `shared_library_check.enabled` | [`pkg/collector/sharedlibrary/sharedlibraryimpl/init.go`](<<<SRC>>>/pkg/collector/sharedlibrary/sharedlibraryimpl/init.go) |

The shared-library loader is experimental: it computes a platform-specific `libdatadog-agent-<name>` shared-object path under `shared_library_check.library_folder_path`, loads it through [`pkg/collector/sharedlibrary/ffi`](<<<SRC>>>/pkg/collector/sharedlibrary/ffi), and drives it through a C-ABI entry point; the Rust check crates live in [`pkg/collector/sharedlibrary/rustchecks`](<<<SRC>>>/pkg/collector/sharedlibrary/rustchecks).

## The collector component

[`comp/collector/collector`](<<<SRC>>>/comp/collector/collector) is a modern def/fx/impl [component](../components/overview.md). Its constructor also initializes Python (`python.InitPython(common.GetPythonPaths()...)`) unless `python_lazy_loading` (default `true`) defers that to the first Python check load, and initializes the shared-library loader when enabled. On Fx `OnStart`, [`collectorImpl.start`](<<<SRC>>>/comp/collector/collector/impl/collector.go) builds a `runner.Runner` and a `scheduler.Scheduler` wired together by two channels (a normal and a shadow check pipe) and starts the scheduler loop.

`RunCheck` wraps every check in [`middleware.CheckWrapper`](<<<SRC>>>/comp/collector/collector/impl/internal/middleware/check_wrapper.go) before scheduling it. The wrapper:

1. Serializes `Run` against `Cancel`: after `Cancel`, it destroys the check's `Sender` (releasing its metric contexts in the aggregator) and prevents any further runs.
1. Injects the health-platform issue reporter into checks implementing `IssueAwareCheck`.
1. Opens an agent-telemetry startup span around each run.

`RunCheck` rejects duplicate check IDs, enters the check into the collector scheduler, and adjusts the worker pool (see below). The component also exposes a status provider and, every 10 minutes, ships the `agent_checks` metadata payload (running checks plus external host tags) built in [`agent_check_metadata.go`](<<<SRC>>>/comp/collector/collector/impl/agent_check_metadata.go).

## Interval scheduling: job queues and buckets

[`scheduler.Enter`](<<<SRC>>>/pkg/collector/scheduler/scheduler.go) enforces a minimum recurring interval of 1 second (`minAllowedInterval`); an interval of 0 means the check is enqueued once, immediately, from a cancellable goroutine. For recurring checks, one [`jobQueue`](<<<SRC>>>/pkg/collector/scheduler/job.go) exists per distinct interval; scheduling a check with a new interval spawns a new queue and goroutine.

A queue with interval N seconds holds N one-second *buckets*. Jobs are placed into buckets using a "sparse step" round-robin so that checks sharing an interval spread across different seconds instead of bursting together. Each queue's own one-second ticker advances its current bucket index, and every check in the current bucket is written to the runner's channel. This write is *blocking*: if all workers are busy, the queue falls behind and logs `Previous bucket took over ...`. Consequently `min_collection_interval` is a minimum, not a guarantee.

Each job queue registers a health handle named `collector-queue-<N>s`, so a stuck runner surfaces in `agent health`. A stopped `scheduler.Scheduler` cannot be restarted — the collector component builds a fresh one on start.

## Runner and workers

[`runner.Runner`](<<<SRC>>>/pkg/collector/runner/runner.go) owns the worker goroutines, which all consume from the shared pending-checks channel. Pool sizing:

1. `check_runners` (default 4) sets a static worker count.
1. `check_runners: 0` enables dynamic sizing based on the number of scheduled checks: up to 10 checks → 4 workers, up to 15 → 10, up to 20 → 15, up to 25 → 20, more → 25 (`MaxNumWorkers`). Dynamic sizing only ever grows the pool; it never shrinks.
1. Each long-running check (`Interval() == 0`) gets one extra dedicated worker, since it pins a worker forever.
1. Each shadow check gets one extra *shadow* worker consuming from the separate shadow channel.
1. `tracemalloc_debug: true` forces `check_runners` to 1, serializing all check execution.

One execution cycle in [`worker.Worker.Run`](<<<SRC>>>/pkg/collector/worker/worker.go):

1. HA gating: if `ha_agent.enabled` is set and the check reports `IsHASupported()` but this agent is not the leader, the run is skipped.
1. Dedup: the running-checks tracker refuses to start a check ID that is already running — a check never runs concurrently with itself; the overlapping run is silently dropped.
1. If `check_watchdog_warning_timeout` is non-zero, a watchdog goroutine logs a warning when the run exceeds it.
1. `check.Run()` executes inside a `recover()`, so a panicking check cannot crash the agent; the panic is converted to a check error.
1. After the run: warnings are collected, the `datadog.agent.check_status` service check is emitted through the *default* sender when `integration_check_status_enabled` is set (note the sender `Commit()` currently happens even when that gate is off — a FIXME in `worker.go`, kept for historical behavior since the default sender is shared), and run stats are recorded into [`expvars.AddCheckStats`](<<<SRC>>>/pkg/collector/runner/expvars/expvars.go) unless the check was unscheduled meanwhile.

## Stopping checks and shutdown

Unscheduling flows from Autodiscovery: `CheckScheduler.Unschedule(configs)` looks up the check IDs recorded for each config digest and calls `collector.StopCheck(id)` for each. `StopCheck`:

1. Removes the check from its job queue (`scheduler.Cancel`); the queue keeps ticking even when empty.
1. Calls `runner.StopCheck(id)`, which invokes `Stop()` if the check is currently running and waits up to 500 ms (`stopCheckTimeout`).
1. Calls `Cancel()` with a `check_cancel_timeout` (default 500 ms) budget.
1. The `CheckWrapper` then destroys the check's sender, releasing its aggregator state.

At agent shutdown the runner allows 2 seconds total for all running checks to stop (`stopAllChecksTimeout`). Python subprocesses started through `get_subprocess_output` are terminated explicitly (see [Python checks](python.md)).

## Long-running checks

Two patterns exist for checks that never "finish":

1. Return `Interval() == 0` and implement `Stop()`. The check is scheduled once and its `Run` occupies a dedicated worker until the agent stops it.
1. Wrap the check in [`corechecks.LongRunningCheckWrapper`](<<<SRC>>>/pkg/collector/corechecks/longrunning.go), which starts the real `Run` once in a background goroutine while reporting a 15-second interval, so the scheduler re-invokes the wrapper periodically merely to `Commit()` the sender and refresh stats. Without this, long-running checks never appear with fresh data on the status page.

See [Go core checks](corechecks.md) for which corechecks use each pattern.

## Shadow checks (metric lookback)

An experimental, feature-flagged lane runs *shadow* copies of selected check instances at a faster interval (default 1 s) for metric-lookback purposes. When `metric_lookback.enabled` is set (optionally scoped by `metric_lookback.enabled_checks` or a per-instance `metric_lookback.enabled`), [`metriclookback.SelectShadowCandidates`](<<<SRC>>>/pkg/collector/metriclookback/policy.go) picks instances at schedule time; each is loaded a second time through a shadow-mode Go loader or the Python loader and wrapped by [`check.ShadowCheck`](<<<SRC>>>/pkg/collector/check/shadow.go) with ID `<source-id>:shadow`. Shadow checks run on dedicated shadow job queues and shadow workers, send through a dedicated `lookbacksender.SenderManager` rather than the main aggregator, and skip the `datadog.agent.check_status` service check.

## Infrastructure mode gating

[`IsCheckAllowed`](<<<SRC>>>/pkg/collector/infra_mode.go) enforces `infrastructure_mode` at schedule time and in the `agent check` CLI. When a mode's `integration.<mode>.allowed` list is non-empty, only listed checks, checks prefixed `custom_`, and `integration.additional` entries may run; `integration.excluded` always wins, and `integration.enabled: false` disables all checks. Eligible checks can also have an infra-mode tagger attached to their senders for tag injection.

## Stats and observability

| Surface | Content |
|---|---|
| expvar `scheduler` | `QueuesCount`, `ChecksEntered`, per-queue stats (interval, buckets, size) |
| expvar `CheckScheduler` | `LoaderErrors`, `RunErrors` (from [`pkg/collector/stats.go`](<<<SRC>>>/pkg/collector/stats.go)) |
| expvar `runner` / `Workers` | Per-check run stats, running checks, worker utilization (from [`pkg/collector/runner/expvars`](<<<SRC>>>/pkg/collector/runner/expvars)) |
| Internal telemetry | `scheduler.checks_entered`, `scheduler.queues_count`, `collector.worker_utilization{worker_name}` |
| Health | `collector-queue-<N>s` liveness handles per job queue |

These feed the Collector section of `agent status` (rendered by [`pkg/status/collector`](<<<SRC>>>/pkg/status/collector)), the [`inventorychecks`](<<<SRC>>>/comp/metadata/inventorychecks) metadata payload, and the `agent_checks` metadata payload. See [Status, health, and telemetry](../operations/introspection.md).

## Configuration

| Key | Default | Effect |
|---|---|---|
| `check_runners` | 4 | Worker count; `0` enables dynamic sizing up to 25 |
| `check_cancel_timeout` | 500ms | Budget for `Cancel()` when unscheduling |
| `check_watchdog_warning_timeout` | 0 (off) | Warn when a single run exceeds this duration |
| `check_runner_utilization_threshold` | 0.95 | Worker utilization warning threshold (with monitor interval and cooldown keys) |
| `integration_check_status_enabled` | false | Gates the `datadog.agent.check_status` service check |
| `integration_tracing`, `integration_tracing_exhaustive` | false | Check tracing, used by `agent check --trace` |
| `infrastructure_mode`, `integration.*` | — | Infrastructure-mode check gating |
| `ha_agent.enabled` | false | Leader-election gating of HA-supported checks |
| `prioritize_go_check_loader` | true | Go loader priority 10 (before Python) instead of 30 |
| `python_lazy_loading` | true | Defer Python interpreter init to first Python check load |
| `shared_library_check.enabled` | false | Enable the experimental Rust shared-library loader |
| `metric_lookback.*` | disabled | Shadow-check selection and interval |
| `telemetry.checks` | — | List of check names with per-check telemetry enabled |
| `tracemalloc_debug` | false | Python tracemalloc; forces `check_runners: 1` |

## Deployment-mode differences

1. Binaries without a collector (for example the standalone DogStatsD image, or builds without the `python` tag) use [`comp/collector/collector/noop-impl`](<<<SRC>>>/comp/collector/collector/noop-impl); the loaders that are compiled out simply never register.
1. The Cluster Agent runs the same pipeline with a four-check Go catalog and no Python by default; *cluster checks* dispatched by the DCA are executed on node agents or cluster-check runners through this very pipeline via the `clusterchecks` Autodiscovery provider — see [Cluster checks and endpoints checks](../containers/cluster-checks.md).
1. The `agent check <name>` CLI ([`pkg/cli/subcommands/check`](<<<SRC>>>/pkg/cli/subcommands/check)) builds an ephemeral collector, runs one check instance a configurable number of times, and prints the aggregator contents instead of forwarding them; see [Diagnostics and CLI tools](../operations/diagnostics.md). It cannot see configurations scheduled through remote config (single-process RC database limitation).

## Gotchas

1. `min_collection_interval` is a floor, not a schedule: bucket dispatch blocks on busy workers, and a check already running is silently skipped by the dedup tracker.
1. Python checks serialize on the CPython GIL regardless of `check_runners`; a fleet of CPU-bound Python checks cannot use more than one core (see [Python checks](python.md)).
1. Dynamic worker sizing never shrinks, and every long-running check pins one worker forever.
1. A config file whose only payload is `logs:` does not create a check; neither does `metrics.yaml` inside a `<check>.d` folder (that file is JMX metric definitions).
1. JMX instances never become `check.Check` objects; their scheduling and status flow through the separate [JMXFetch subsystem](jmx.md).
1. Editing a check config changes the check ID (digest-based), so dashboards keyed on check IDs see a new instance.
1. The worker's `sender.Commit()` runs even when `integration_check_status_enabled` is false; because the default sender is shared, this also flushes other pending data on it.
