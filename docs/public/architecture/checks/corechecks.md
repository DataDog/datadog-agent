# Go core checks

-----

Go core checks (or "corechecks") are check implementations compiled into the agent binary itself, as opposed to [Python checks](python.md) loaded at runtime or [JMX checks](jmx.md) run in a JVM sidecar. They live under [`pkg/collector/corechecks`](<<<SRC>>>/pkg/collector/corechecks), implement the same [`check.Check`](<<<SRC>>>/pkg/collector/check/check.go) interface as every other check, and are loaded by the `core` loader from a per-binary factory catalog. Because they cannot be upgraded independently of the agent, `Version()` returns an empty string for them.

How checks are scheduled and executed is covered in [Check collector](collector.md); this page covers what a Go check looks like, where the catalog lives, and how the corechecks tree is organized.

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/collector/corechecks/loader.go`](<<<SRC>>>/pkg/collector/corechecks/loader.go) | `GoCheckLoader` (loader name `core`) and the global factory catalog (`RegisterCheck`, `RegisterContextualCheck`) |
| [`pkg/collector/corechecks/checkbase.go`](<<<SRC>>>/pkg/collector/corechecks/checkbase.go) | `CheckBase`: default implementation of most of `check.Check` |
| [`pkg/collector/corechecks/safesender.go`](<<<SRC>>>/pkg/collector/corechecks/safesender.go) | Safe sender wrapper returned by `CheckBase.GetSender()` |
| [`pkg/collector/corechecks/longrunning.go`](<<<SRC>>>/pkg/collector/corechecks/longrunning.go) | `LongRunningCheckWrapper` for checks whose `Run` never returns |
| [`pkg/commonchecks/corechecks.go`](<<<SRC>>>/pkg/commonchecks/corechecks.go) | Registration of every Go check factory for the agent flavor |
| [`pkg/commonchecks/corechecks_cluster.go`](<<<SRC>>>/pkg/commonchecks/corechecks_cluster.go) | Cluster Agent catalog (4 checks) |
| [`pkg/commonchecks/corechecks_sysprobe.go`](<<<SRC>>>/pkg/commonchecks/corechecks_sysprobe.go) | eBPF check factories (`systemprobechecks` build tag) |
| [`comp/checks`](<<<SRC>>>/comp/checks) | Windows-only component-based checks bundle |
| [`pkg/collector/check/defaults`](<<<SRC>>>/pkg/collector/check/defaults) | `DefaultCheckInterval` (15 s) |

## Anatomy of a Go check

A Go check is a struct embedding [`corechecks.CheckBase`](<<<SRC>>>/pkg/collector/corechecks/checkbase.go), which supplies default implementations for almost the entire `check.Check` interface. A minimal check provides three things: a factory, a `Configure` method, and a `Run` method.

```go
type MyCheck struct {
    core.CheckBase
    instanceCfg myInstanceConfig
}

func Factory() option.Option[func() check.Check] {
    return option.New(func() check.Check {
        return &MyCheck{CheckBase: core.NewCheckBase(CheckName)}
    })
}

func (c *MyCheck) Configure(senderManager sender.SenderManager, digest uint64, rawInstance, rawInitConfig integration.Data, source, provider string) error {
    // multi-instance checks MUST build a unique ID before anything else
    c.BuildID(digest, rawInstance, rawInitConfig)
    if err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source, provider); err != nil {
        return err
    }
    return yaml.Unmarshal(rawInstance, &c.instanceCfg)
}

func (c *MyCheck) Run() error {
    snd, err := c.GetSender()
    if err != nil {
        return err
    }
    snd.Gauge("my.metric", 1.0, "", nil)
    snd.Commit()
    return nil
}
```

What `CheckBase` handles for you:

1. `NewCheckBase(name)` sets the default 15-second interval (`defaults.DefaultCheckInterval`) and defaults the check ID to the bare check name — correct only for single-instance checks. Checks that support multiple instances must call `BuildID(digest, instance, initConfig)` from `Configure` so each instance gets a distinct `name:hexhash` ID. `NewCheckBaseWithInterval` overrides the default interval.
1. `CommonConfigure` parses the common instance fields understood by the framework rather than the check: `min_collection_interval` (seconds), `empty_default_hostname`, `tags` (appended to every submission), `service`, and `no_index`. It must be called from your `Configure` (or you inherit `CheckBase.Configure`, which calls it).
1. `GetSender()` returns a *safe sender* ([`safesender.go`](<<<SRC>>>/pkg/collector/corechecks/safesender.go)) that guards against common sender misuse at a small cost; performance-critical checks can use `GetRawSender()`.
1. `Warn`/`Warnf` record integration warnings that surface in logs, `agent status`, and the web UI.
1. Default no-op `Stop()` and `Cancel()`; override `Cancel` if the check owns background resources, and override both `Stop` and `Interval` for long-running checks.

The check pushes data through the `sender.Sender` API (`Gauge`, `Rate`, `MonotonicCount`, `ServiceCheck`, `Event`, ...) and must call `Commit()` once per run; the sender routes into the [aggregator](../pipelines/metrics/aggregation.md) keyed by check ID.

## Registration: the per-binary catalog

Factories are not registered via `init()` side effects; each binary explicitly calls [`pkg/commonchecks.RegisterChecks(...)`](<<<SRC>>>/pkg/commonchecks/corechecks.go) at startup, passing the components that checks need (workloadmeta, tagger, workload filter, telemetry, remote-config client, and so on). This makes the catalog an explicit, per-binary decision:

1. The core agent registers the full catalog in [`corechecks.go`](<<<SRC>>>/pkg/commonchecks/corechecks.go). Several registrations are conditional on the platform (files with `//go:build windows` and friends inside each check package decide whether `Factory()` returns a real factory or none).
1. The Cluster Agent build (`clusterchecks && kubeapiserver` tags) uses [`corechecks_cluster.go`](<<<SRC>>>/pkg/commonchecks/corechecks_cluster.go), which registers only `kubernetes_apiserver`, `kubernetes_state_core`, `helm`, and `orchestrator`. Registering a check in `corechecks.go` does nothing for the DCA.
1. The `systemprobechecks` build tag adds the eBPF-backed checks (`ebpf`, `oom_kill`, `tcp_queue_length`, `noisy_neighbor`) via [`corechecks_sysprobe.go`](<<<SRC>>>/pkg/commonchecks/corechecks_sysprobe.go).

`RegisterCheck(name, factory)` fills the global catalog consulted by [`GoCheckLoader.Load`](<<<SRC>>>/pkg/collector/corechecks/loader.go): the loader looks up `config.Name`, invokes the factory, and calls `Configure` on the fresh instance. `RegisterContextualCheck` registers a mode-aware factory receiving a `ConstructionContext` (`NormalLoadMode` vs `ShadowLoadMode`) for checks that must behave differently as metric-lookback shadow copies (see [Check collector](collector.md)).

A separate family of Windows-only checks is built as Fx components in [`comp/checks`](<<<SRC>>>/comp/checks) (`agentcrashdetect`, `windowseventlog`, `winregistry`); they hook the same factory catalog but participate in the [component framework](../components/overview.md) to receive their dependencies.

## Taxonomy of pkg/collector/corechecks

| Directory | Checks | Notes |
|---|---|---|
| [`system`](<<<SRC>>>/pkg/collector/corechecks/system) | `cpu`, `load`, `memory`, `uptime`, `io`, `disk`, `file_handle`, `battery`, `thermal`, `wincrashdetect`, `windows_certificate`, `winkmem`, `winproc` | `use_diskv2_check` switches `disk` to the pure-Go `diskv2` rewrite |
| [`net`](<<<SRC>>>/pkg/collector/corechecks/net) | `network`, `ntp`, `wlan` | `use_networkv2_check` switches to `networkv2`; `wlan` is macOS/Windows |
| [`cluster`](<<<SRC>>>/pkg/collector/corechecks/cluster) | `kubernetes_apiserver`, `kubernetes_state_core` (KSM), `helm`, `orchestrator` | Also the entire [Cluster Agent](../containers/cluster-agent.md) catalog |
| [`containers`](<<<SRC>>>/pkg/collector/corechecks/containers) | `container` (generic), `containerd`, `cri`, `docker`, `kubelet`, `kata_containers`, `datadog_csi_driver` | Driven by [workloadmeta](../containers/workloadmeta.md) and the tagger |
| [`containerimage`](<<<SRC>>>/pkg/collector/corechecks/containerimage), [`containerlifecycle`](<<<SRC>>>/pkg/collector/corechecks/containerlifecycle), [`sbom`](<<<SRC>>>/pkg/collector/corechecks/sbom) | `container_image`, `container_lifecycle`, `sbom` | Long-running, workloadmeta-event-driven |
| [`orchestrator`](<<<SRC>>>/pkg/collector/corechecks/orchestrator) | `orchestrator_pod`, `orchestrator_ecs`, `orchestrator_kubelet_config` | Node-side [orchestrator explorer](../containers/orchestrator.md) collection |
| [`ebpf`](<<<SRC>>>/pkg/collector/corechecks/ebpf) | `ebpf`, `oom_kill`, `tcp_queue_length`, `noisy_neighbor` | Thin clients querying [system-probe](../ebpf/system-probe.md) modules over its socket; see the package's [`AGENTS.md`](<<<SRC>>>/pkg/collector/corechecks/ebpf/AGENTS.md) |
| [`embed`](<<<SRC>>>/pkg/collector/corechecks/embed) | `apm`, `process_agent` | Watchdog-style checks observing the other agent processes |
| [`gpu`](<<<SRC>>>/pkg/collector/corechecks/gpu), [`nvidia`](<<<SRC>>>/pkg/collector/corechecks/nvidia) | `gpu`, `nccl`, `jetson` | GPU telemetry |
| [`network-devices`](<<<SRC>>>/pkg/collector/corechecks/network-devices) | `cisco_sdwan`, `versa` | See [NDM servers](../pipelines/ndm.md) |
| [`snmp`](<<<SRC>>>/pkg/collector/corechecks/snmp) | `snmp` | Consumes remote config directly for device profiles |
| [`networkpath`](<<<SRC>>>/pkg/collector/corechecks/networkpath) | `network_path` | Traceroute-based path monitoring |
| [`networkconfigmanagement`](<<<SRC>>>/pkg/collector/corechecks/networkconfigmanagement) | `network_config_management` | Network device config retrieval |
| [`oracle`](<<<SRC>>>/pkg/collector/corechecks/oracle) | `oracle` / `oracle-dbm` | DBM check; both names map to the same factory |
| [`systemd`](<<<SRC>>>/pkg/collector/corechecks/systemd) | `systemd` | Linux only |
| [`cloud`](<<<SRC>>>/pkg/collector/corechecks/cloud) | `cloud_hostinfo` | Cloud host metadata |
| [`discovery`](<<<SRC>>>/pkg/collector/corechecks/discovery) | `discovery` | Process/service discovery |
| [`telemetry`](<<<SRC>>>/pkg/collector/corechecks/telemetry) | `telemetry` | Exposes agent-internal telemetry as metrics |
| [`agentprofiling`](<<<SRC>>>/pkg/collector/corechecks/agentprofiling) | `agentprofiling` | Triggers agent profiles/flares |

## Long-running checks

Checks whose collection is event-driven rather than interval-driven (for example `container_image`, which reacts to workloadmeta events) use one of two patterns, described in detail in [Check collector](collector.md):

1. Report `Interval() == 0` and implement `Stop()`: the check is scheduled once and monopolizes one worker.
1. Implement `LongRunningCheck` (adding `GetSender()`) and wrap the factory result in [`NewLongRunningCheckWrapper`](<<<SRC>>>/pkg/collector/corechecks/longrunning.go): the wrapper starts the real `Run` in a background goroutine once, then reports a 15-second interval so periodic re-invocations commit the sender and keep status-page stats fresh. Note the wrapper never restarts a `Run` that returned — long-running checks are not meant to terminate.

## Coexistence with Python implementations

Several integrations exist in both Go and Python (`disk`, `network`, and others in transition). The mechanics:

1. With `prioritize_go_check_loader: true` (the default), the `core` loader runs at priority 10 and wins over Python (priority 20). An integration present in both catalogs runs the Go version unless the Go factory refuses.
1. A Go check can decline a particular configuration by returning `check.ErrSkipCheckInstance` from `Configure`, letting the config fall through to the Python loader. Python checks signal the same thing with a specific error message matched by string (see [Python checks](python.md)).
1. A config can force a specific implementation with `loader: python` or `loader: core` in `init_config` or the instance.
1. Some migrations are gated by dedicated flags instead (`use_diskv2_check`, `use_networkv2_check`) that decide which Go factory gets registered under the shared name.

## Adding a new Go check

1. Create a package under the appropriate [`pkg/collector/corechecks`](<<<SRC>>>/pkg/collector/corechecks) subdirectory exporting `CheckName` and `Factory() option.Option[func() check.Check]`; platform-conditional checks return `option.None` on unsupported platforms via build-tagged files.
1. Embed `CheckBase`, implement `Configure` (calling `BuildID` when multi-instance, then `CommonConfigure`) and `Run`.
1. Register the factory in [`pkg/commonchecks/corechecks.go`](<<<SRC>>>/pkg/commonchecks/corechecks.go) (and `corechecks_cluster.go` if the Cluster Agent must run it).
1. Ship a default config: add `cmd/agent/dist/conf.d/<name>.d/conf.yaml.default` (or `conf.yaml.example` for opt-in checks) so packaging picks it up.
1. Add the package to the build (Bazel `BUILD.bazel` files are generated; run the repo's Gazelle task) and write tests runnable with `dda inv test --targets=./pkg/collector/corechecks/<dir>`.

## Gotchas

1. Forgetting `BuildID` in a multi-instance check makes all instances share one ID; the collector rejects the duplicates and only one instance runs.
1. `CheckBase.Configure` and `CommonConfigure` fetch the sender to apply tags/service/hostname settings, which implicitly *creates* the sender for the check ID — call them only after `BuildID`.
1. The default interval is 15 s even if your check documents another cadence; set it via `NewCheckBaseWithInterval` or rely on `min_collection_interval` in the config.
1. Factories run once per instance per schedule event: keep them cheap, and do the heavy lifting in `Configure`/first `Run`.
1. Warnings accumulate until `GetWarnings()` drains them after each run; `Warn` on a hot path floods `agent status`.
1. A check registered only in `corechecks.go` silently does not exist in the Cluster Agent, `agent check <name>` included — the DCA catalog is separate by design.
