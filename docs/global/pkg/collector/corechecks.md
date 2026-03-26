> **TL;DR:** Home for all built-in Go checks compiled into the agent binary, providing `CheckBase` and `GoCheckLoader` as the foundation every core check builds on, plus a large library of domain-specific sub-packages covering system, container, cluster, eBPF, and network metrics.

# pkg/collector/corechecks

## Purpose

`pkg/collector/corechecks` is the home for all built-in Go checks (also called "core checks") that ship with the Datadog Agent. It provides:

- The base types and loader that every Go check builds on (`CheckBase`, `GoCheckLoader`, the check catalog).
- A growing library of domain-specific sub-packages, each implementing one or more named checks that collect metrics and surface them through the Datadog aggregator pipeline.

Core checks are distinct from Python-based checks: they are compiled into the agent binary, cannot be installed independently, and carry no version string. They are loaded with priority 30 by default (overridable via `prioritize_go_check_loader`).

## Key Elements

### Key types

### Root package (`pkg/collector/corechecks`)

| Symbol | Kind | Description |
|--------|------|-------------|
| `CheckBase` | struct | Embeddable base that satisfies most of `check.Check`. Provides `Configure`, `CommonConfigure`, `BuildID`, `GetSender`, `GetRawSender`, `Warn`/`Warnf`, and lifecycle stubs (`Stop`, `Cancel`, `Interval`). |
| `NewCheckBase(name)` | func | Returns a `CheckBase` with the default collection interval. |
| `NewCheckBaseWithInterval(name, d)` | func | Returns a `CheckBase` with a custom interval. |
| `LongRunningCheck` | interface | Extends `check.Check` for checks whose `Run()` never returns. |
| `LongRunningCheckWrapper` | struct | Wraps a `LongRunningCheck` so the scheduler sees it as a regular check: starts `Run()` in a goroutine on first tick, commits the sender on subsequent ticks, and guards `Cancel()` against double-calls. |
| `safeSender` | struct (internal) | Wraps `sender.Sender` to copy tag slices before forwarding, preventing in-place mutation bugs. `GetSender()` returns this; `GetRawSender()` bypasses it. |
| `CheckFactory` | type alias | `func() check.Check` — the factory signature registered in the catalog. |
| `RegisterCheck(name, option.Option[CheckFactory])` | func | Adds a check to the in-memory catalog. Must be called at init time (typically from `pkg/commonchecks`). |
| `GoCheckLoaderName` | const | `"core"` — the loader name reported by every core check. |
| `GoCheckLoader` | struct | Implements `check.Loader`. Looks up a factory by check name, instantiates the check, and calls `Configure`. Registered with priority 30 (10 when `prioritize_go_check_loader` is true). |

**Implementing a new core check**

1. Embed `CheckBase` and call `NewCheckBase(CheckName)` from the factory.
2. Override `Configure()` (call `BuildID` for multi-instance checks, then `CommonConfigure`).
3. Implement `Run()` to collect metrics via `GetSender()`.
4. Register with `corecheckLoader.RegisterCheck(CheckName, Factory())` in `pkg/commonchecks/corechecks.go`.

---

### Sub-packages by domain

#### System (`system/`)

OS-level host metrics. All platform-generic; Windows-only checks use build tags.

| Sub-package | Check name | Metrics |
|-------------|------------|---------|
| `system/cpu/cpu` | `cpu` | CPU utilisation (user, system, idle, …) |
| `system/cpu/load` | `load` | CPU load averages |
| `system/memory` | `memory` | RAM and swap usage |
| `system/disk/disk` | `disk` | Disk space per partition |
| `system/disk/diskv2` | `disk` | Alternative disk check (enabled via `use_diskv2_check`) |
| `system/disk/io` | `io` | Disk I/O throughput and latency |
| `system/uptime` | `uptime` | Host uptime |
| `system/filehandles` | `file_handle` | Open file descriptor counts |
| `system/battery` | `battery` | Battery charge level (macOS/Linux) |
| `system/wincrashdetect` | `win32_event_log` | Windows crash-dump detection |
| `system/windowscertificate` | `windows_certificate` | Windows certificate store expiry |
| `system/winkmem` | `winkmem` | Windows kernel memory |
| `system/winproc` | `winproc` | Windows process metrics |

#### Containers (`containers/`)

Container runtime checks. Each takes `workloadmeta`, a workload filter, and a tagger component.

| Sub-package | Check name | Runtime |
|-------------|------------|---------|
| `containers/docker` | `docker` | Docker Engine |
| `containers/containerd` | `containerd` | containerd |
| `containers/cri` | `cri` | CRI-compatible runtimes |
| `containers/kubelet` | `kubelet` | Kubelet metrics endpoint |
| `containers/generic` | `container` | Generic container metrics via workloadmeta |

#### Cluster (`cluster/`)

Kubernetes control-plane checks, used primarily by the Cluster Agent.

| Sub-package | Check name | What it collects |
|-------------|------------|-----------------|
| `cluster/kubernetesapiserver` | `kubernetes_apiserver` | API server events and component statuses |
| `cluster/ksm` | `kubernetes_state` | Kube-State-Metrics (KSM) |
| `cluster/helm` | `helm` | Helm release statuses |
| `cluster/orchestrator` | `orchestrator` | Kubernetes cluster-level orchestrator payload |

#### eBPF (`ebpf/`)

Kernel-space checks that run a probe inside `system-probe` and expose metrics via HTTP. See `pkg/collector/corechecks/ebpf/AGENTS.md` for the full development guide.

| Sub-package | Check name | Build tags | What it monitors |
|-------------|------------|------------|-----------------|
| `ebpf` | `ebpf` | `linux` | eBPF program stats (maps, progs) |
| `ebpf/oomkill` | `oom_kill` | `linux` | OOM kill events per container |
| `ebpf/tcpqueuelength` | `tcp_queue_length` | `linux` | TCP send/receive queue saturation |
| `ebpf/noisyneighbor` | `noisy_neighbor` | `linux` | Container CPU steal / noisy-neighbour |

Each eBPF check has three parts: an agent-side check (this package), a probe inside `pkg/collector/corechecks/ebpf/probe/<check>/`, and a system-probe module in `cmd/system-probe/modules/`. The agent check communicates with the probe over localhost HTTP.

#### GPU (`gpu/`)

CUDA/GPU metrics collected via the `system-probe` GPU module.

| Sub-package | Check name | Build tags |
|-------------|------------|------------|
| `gpu` | `gpu` | `linux` (stub on other platforms) |

Takes `tagger`, `telemetry`, and `workloadmeta` components. Metrics include active GPU processes, memory usage, and utilization per container.

#### SNMP (`snmp/`)

SNMP polling check with autodiscovery support. Accepts a `config.Component`, `rcclient.Component`, and `snmpscanmanager.Component`. Profiles are stored in `snmp/internal/profile/`.

#### Network devices (`network-devices/`)

Vendor-specific network device checks built on top of the SNMP infrastructure:

- `network-devices/cisco-sdwan` — Cisco SD-WAN metrics and topology.
- `network-devices/versa` — Versa Networks SD-WAN.

#### Embed (`embed/`)

Checks that launch and manage sub-processes embedded in the agent:

- `embed/apm` — Manages the trace-agent process.
- `embed/process` — Manages the process-agent process.

Both use `LongRunningCheckWrapper`.

#### Orchestrator (`orchestrator/`)

Payload-generating checks for the Process/Orchestrator backend:

- `orchestrator/pod` — Kubernetes pod manifests.
- `orchestrator/ecs` — ECS task/container metadata.
- `orchestrator/kubeletconfig` — Kubelet configuration snapshot.

#### Other notable packages

| Package | Check name | Notes |
|---------|------------|-------|
| `net/ntp` | `ntp` | NTP clock offset |
| `net/network` / `net/networkv2` | `network` | Network interface metrics; `networkv2` selected via `use_networkv2_check` |
| `net/wlan` | `wlan` | Wi-Fi signal metrics |
| `networkpath` | `network_path` | Traceroute-style path metrics |
| `networkconfigmanagement` | `network_config_management` | Network configuration management |
| `sbom` | `sbom` | Software Bill of Materials collection |
| `containerimage` | `container_image` | Container image metadata |
| `containerlifecycle` | `container_lifecycle` | Container lifecycle events |
| `discovery` | `discovery` | Service discovery |
| `cloud/hostinfo` | `cloud_foundry_bbs` | Cloud host info (CF, etc.) |
| `nvidia/jetson` | `jetson` | Jetson board GPU/thermal metrics |
| `oracle` | `oracle` / `oracle_dbm` | Oracle database metrics |
| `systemd` | `systemd` | systemd unit states |
| `telemetry` | `datadog_agent` | Internal agent telemetry |
| `agentprofiling` | `agent_profiling` | On-demand agent pprof dumps |

---

## Usage

### Registration

All checks are registered at agent startup via `pkg/commonchecks/corechecks.go`:

```go
// pkg/commonchecks/corechecks.go
func RegisterChecks(store workloadmeta.Component, ...) {
    corecheckLoader.RegisterCheck(cpu.CheckName, cpu.Factory())
    corecheckLoader.RegisterCheck(snmp.CheckName, snmp.Factory(cfg, rcClient, snmpScanManager))
    // ...
}
```

eBPF checks that require system-probe are registered separately in `corechecks_sysprobe.go` behind the `systemprobechecks` build tag.

Cluster Agent-only checks (KSM, helm, kubernetes_apiserver) are in `corechecks_cluster.go` behind `clusterchecks && kubeapiserver`.

### Loading

`GoCheckLoader.Load()` is called by the collector when autodiscovery resolves a matching integration config. It looks up the factory in the catalog, instantiates the check, and calls `Configure`. The loader is registered automatically via `init()` in `loader.go`.

### Writing a check

```go
// Example minimal check skeleton
type MyCheck struct {
    corechecks.CheckBase
}

const CheckName = "my_check"

func Factory() option.Option[func() check.Check] {
    return option.NewWithValue(func() check.Check {
        return &MyCheck{CheckBase: corechecks.NewCheckBase(CheckName)}
    })
}

func (c *MyCheck) Run() error {
    sender, err := c.GetSender()
    if err != nil {
        return err
    }
    sender.Gauge("my_check.metric", 42.0, "", nil)
    sender.Commit()
    return nil
}
```

Then add `corecheckLoader.RegisterCheck(mycheck.CheckName, mycheck.Factory())` to `pkg/commonchecks/corechecks.go`.

### Build tags summary

| Tag | Effect |
|-----|--------|
| `linux` | Enables Linux-only checks (most eBPF checks) |
| `linux_bpf` | Enables the actual eBPF probe implementation |
| `systemprobechecks` | Enables system-probe-backed checks |
| `kubeapiserver` | Enables cluster-agent checks |
| `clusterchecks && kubeapiserver` | Enables Cluster Agent exclusive checks |

The `registry_kubeapiserver.go` file in `bundles/` (privateactionrunner) uses a similar pattern: absence of `kubeapiserver` tag selects the default registry.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/collector/check`](check.md) | Defines the `Check` and `Loader` interfaces that `CheckBase` and `GoCheckLoader` satisfy. `corechecks.CheckBase` is the canonical embedding that gives Go checks a compliant `Check` implementation for free. |
| [`pkg/collector/loaders`](loaders.md) | `GoCheckLoader` registers itself here (priority 30, or 10 with `prioritize_go_check_loader`). The scheduler iterates loaders in priority order; `PythonCheckLoader` (priority 20) runs first unless the config specifies `loader: core`. |
| [`pkg/collector/python`](python.md) | Python checks run alongside core checks through the same loader chain. When both a Python and a Go version of a check exist, the Go loader returns `ErrSkipCheckInstance` for configs the Python version should handle (e.g. JMX variants). |
| [`pkg/aggregator/sender`](../aggregator/sender.md) | `CheckBase.GetSender()` delegates to `SenderManager.GetSender(c.ID())`. The `safeSender` wrapper copies tag slices before forwarding to prevent mutation bugs. `DestroySender` is called automatically by the collector on unschedule. |
| [`pkg/ebpf`](../ebpf.md) | eBPF core checks (`ebpf/`, `ebpf/oomkill`, `ebpf/tcpqueuelength`, etc.) communicate with probes in `pkg/collector/corechecks/ebpf/probe/<check>/` and with system-probe modules over localhost HTTP. The full development guide is at `pkg/collector/corechecks/ebpf/AGENTS.md`. |
| [`pkg/snmp`](../snmp.md) | The `snmp/` sub-package in corechecks consumes `pkg/snmp` utilities for configuration parsing, credential management, and device digests. |
