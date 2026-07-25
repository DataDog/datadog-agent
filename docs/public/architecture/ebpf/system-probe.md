# system-probe

-----

system-probe is the privileged sibling process of the core Agent. It hosts every data-collection capability that needs more privileges than the `dd-agent` user has: eBPF programs on Linux, kernel drivers on Windows, raw sockets, privileged `/proc` access, and cross-container inspection. Rather than being one monolithic feature, system-probe is a **module registry**: each capability (network tracing, universal service monitoring, the CWS event monitor, OOM-kill tracking, GPU monitoring, traceroute, service discovery, and more) is a self-registering factory that is instantiated only when its configuration flag enables it. Modules expose HTTP endpoints on a local socket that the core agent, process-agent, and security-agent poll. This page covers the process model, the module system, configuration, eBPF loading strategies, IPC, and privileges; the individual data products are covered on [Network monitoring (NPM and USM)](network-monitoring.md), [Workload Protection (CWS)](cws.md), and [Compliance and SBOM](compliance.md).

## Why a separate process

The core agent runs as an unprivileged user (`dd-agent` on Linux, `ddagentuser` on Windows; see [Binaries and flavors](../processes/binaries.md)). Loading eBPF programs, reading tracefs, opening raw sockets, and attaching uprobes to arbitrary processes all require root or targeted capabilities. Isolating that work in system-probe keeps the privileged surface small, lets kernel-level code crash without taking down metric collection, and gives the kernel-facing code its own configuration file (`system-probe.yaml`) and lifecycle. system-probe depends on the core agent for *metadata* — it consumes the [tagger](../containers/tagger.md), [workloadmeta](../containers/workloadmeta.md), and hostname remotely over the core agent's gRPC IPC (see [Inter-process communication](../processes/ipc.md)) — but performs all data collection itself.

```text
                 datadog.yaml                          system-probe.yaml
                      |                                       |
   +------------------+-----------+       +------------------+------------------+
   | core agent (dd-agent user)   |       | system-probe (root / LocalSystem)   |
   |  USM check, eBPF checks,     |       |  +-------------------------------+  |
   |  discovery, GPU check, ...   |------>|  | module HTTP API               |  |
   +------------------------------+  UDS  |  |  /network_tracer/*  /gpu/*    |  |
   | process-agent (dd-agent)     | socket|  |  /discovery/*  /traceroute/*  |  |
   |  connections + process stats |  or   |  |  /debug/stats  ...            |  |
   +------------------------------+ named |  +-------------------------------+  |
   | security-agent (CWS rules UI)| pipe  |  eBPF programs / Windows drivers    |
   +------------------------------+       +------------------+------------------+
              ^                                              |
              | gRPC (runtime-security.sock): CWS events     |
              +----------------------------------------------+
   core agent gRPC IPC (localhost:5001): remote tagger, workloadmeta,
   hostname, config sync  <---- system-probe is a client here
```

## Key packages

| Path | Purpose |
|---|---|
| [`cmd/system-probe`](<<<SRC>>>/cmd/system-probe) | Entrypoints and subcommands (`run`, `config`, `debug`, `modrestart`, ...) |
| [`cmd/system-probe/subcommands/run/command.go`](<<<SRC>>>/cmd/system-probe/subcommands/run/command.go) | Fx wiring, signal handling, `startSystemProbe` |
| [`cmd/system-probe/api/server.go`](<<<SRC>>>/cmd/system-probe/api/server.go) | `StartServer`: listener, module registration, global endpoints |
| [`cmd/system-probe/modules`](<<<SRC>>>/cmd/system-probe/modules) | All module factories, one file per module; `modules.go` holds the init order |
| [`pkg/system-probe/api/module`](<<<SRC>>>/pkg/system-probe/api/module) | `Module`/`Factory` types, the loader, per-module routers, restart support |
| [`pkg/system-probe/api/server`](<<<SRC>>>/pkg/system-probe/api/server) | UDS listener (Linux/macOS) and named-pipe listener (Windows) |
| [`pkg/system-probe/api/client`](<<<SRC>>>/pkg/system-probe/api/client) | Client used by the other agent processes to call module endpoints |
| [`pkg/system-probe/config`](<<<SRC>>>/pkg/system-probe/config) | Config loading, normalization (`adjust*.go`), and the module enablement matrix |
| [`pkg/config/setup/system_probe.go`](<<<SRC>>>/pkg/config/setup/system_probe.go) | All `system-probe.yaml` defaults and `DD_*` env bindings |
| [`pkg/ebpf`](<<<SRC>>>/pkg/ebpf) | eBPF loading infrastructure: CO-RE, BTF sourcing, runtime compilation, manager wrappers, telemetry |
| [`pkg/network`](<<<SRC>>>/pkg/network) | NPM and USM implementation (see [Network monitoring](network-monitoring.md)) |
| [`pkg/eventmonitor`](<<<SRC>>>/pkg/eventmonitor) | Event monitor module core, wrapping the CWS probe (see [CWS](cws.md)) |
| [`pkg/collector/corechecks/ebpf`](<<<SRC>>>/pkg/collector/corechecks/ebpf) | Probe implementations and agent-side checks for oom_kill, tcp_queue_length, ebpf, noisy_neighbor |
| [`pkg/discovery/module`](<<<SRC>>>/pkg/discovery/module) | Service discovery module (plus the `system-probe-lite` variant) |
| [`pkg/gpu`](<<<SRC>>>/pkg/gpu) | GPU monitoring probe (CUDA uprobes plus NVML) |
| [`pkg/dyninst/module`](<<<SRC>>>/pkg/dyninst/module) | Dynamic Instrumentation for Go |
| [`pkg/network/driver`](<<<SRC>>>/pkg/network/driver) | Windows `ddnpm` kernel driver interface |
| [`pkg/windowsdriver`](<<<SRC>>>/pkg/windowsdriver) | Windows `ddprocmon` driver plumbing used by the event monitor |

## Process lifecycle

1. [`main.go`](<<<SRC>>>/cmd/system-probe/main.go) sets the flavor to `SystemProbe` and dispatches cobra subcommands. On Windows, [`main_windows.go`](<<<SRC>>>/cmd/system-probe/main_windows.go) detects service mode and runs through `servicemain` as the `datadog-system-probe` service.
1. The `run` subcommand ([`command.go`](<<<SRC>>>/cmd/system-probe/subcommands/run/command.go)) builds an Fx app. Notable dependencies: the sysprobeconfig component, a **remote** workloadmeta and **remote** tagger (both backed by the core agent's gRPC IPC), `ipcfx.ModuleReadWrite` (auth token and TLS certs for talking to the core agent), a Remote Config client (`AgentName: "system-probe"`), a statsd client, health probe, runtime settings, configsync, and — for direct send — the event-platform and connections forwarders.
1. Before starting anything, `run()` checks whether it should exec into `system-probe-lite`: if `discovery.use_system_probe_lite` is set and discovery is the *only* enabled module, it `syscall.Exec`s into the slimmer `system-probe-lite` binary next to the main one ([`splite.go`](<<<SRC>>>/cmd/system-probe/subcommands/run/splite.go)). The PID stays the same but the running code is a different binary.
1. `run()` also disables transparent huge pages for its own process (`system_probe_config.disable_thp`, default true) and installs SIGTERM/SIGPIPE handling, then calls `startSystemProbe`.
1. `startSystemProbe` exits with `ErrNotEnabled` if no module is enabled — after sleeping five seconds, deliberately, so systemd/supervisord does not treat the immediate exit as a crash loop. Otherwise it sets up core dumps, the optional cgroup memory monitor (`system_probe_config.memory_controller.*`), internal profiling, and an expvar/pprof/telemetry HTTP server on `127.0.0.1:<system_probe_config.debug_port>` (only when the port is set), then calls `api.StartServer`.
1. [`StartServer`](<<<SRC>>>/cmd/system-probe/api/server.go) creates the UDS or named-pipe listener, registers all enabled modules via `module.Register`, and adds the global endpoints: `/debug/stats` (also the readiness probe used by clients), `POST /module-restart/{module_name}`, `GET /debug/pprof/`, `/debug/vars`, `/telemetry`, the Linux-only `/debug/ebpf_btf_loader_info`, `/debug/dmesg` and `/debug/selinux_*` helpers, and `POST /agent-restart` — the only endpoint wrapped in the IPC auth-token middleware.

On Linux host installs the systemd unit [`datadog-agent-sysprobe.service`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm/datadog-agent-sysprobe.service) runs the binary as root (no `User=` directive) with `Requires=sys-kernel-debug.mount` (kprobes need tracefs) and `BindsTo=datadog-agent.service`, so system-probe stops when the core agent stops. See [Process supervision](../processes/supervision.md) for how the units relate.

## Module registry and loader

A module is described by a [`module.Factory`](<<<SRC>>>/pkg/system-probe/api/module/factory_linux.go): a name, a constructor `Fn`, and on Linux the `NeedsEBPF`/`OptionalEBPF` hints. Each module file in [`cmd/system-probe/modules`](<<<SRC>>>/cmd/system-probe/modules) self-registers in `init()`; [`modules.go`](<<<SRC>>>/cmd/system-probe/modules/modules.go) then sorts the registered factories by an explicit `moduleOrder` slice. The order is load-bearing: the event monitor must start **after** the network tracer (it checks `module.IsLoaded(config.NetworkTracerModule)` to decide whether to attach the network consumer), and Dynamic Instrumentation and GPU must start **after** the event monitor (GPU consumes the process-event consumer created by the event monitor factory, passed via a package-global variable in [`gpu.go`](<<<SRC>>>/cmd/system-probe/modules/gpu.go) — an acknowledged hack).

[`module.Register`](<<<SRC>>>/pkg/system-probe/api/module/loader.go) drives startup:

1. Filter the factory list by `cfg.ModuleIsEnabled`.
1. Run `preRegister`: on Linux ([`loader_linux.go`](<<<SRC>>>/pkg/system-probe/api/module/loader_linux.go)) this calls `ebpf.Setup()` if any enabled module needs eBPF — initializing the CO-RE/BTF loaders and removing the memlock rlimit process-wide.
1. Instantiate each factory inside `pprof.Do` with a `module=<name>` label, so CPU profiles are attributable per module.
1. Mount each module's HTTP routes on a per-module `Router` under `/{module_name}/`.
1. Run `postRegister`, which flushes cached BTF — freeing what can be hundreds of MB used only during program loading.

A module that fails to start **does not abort system-probe**: the error is recorded and surfaced in `/debug/stats` (and in `agent status`). Only if *every* enabled module fails does `Register` return an error and the process exit. Every 15 seconds each module's `GetStats()` is collected into the map served at `/debug/stats` and published as the `modules` expvar. Modules can be recreated in place via `POST /module-restart/{module_name}`.

## Module enablement matrix

[`load()`](<<<SRC>>>/pkg/system-probe/config/config.go) computes the set of enabled modules from `system-probe.yaml` (and a few `datadog.yaml` keys). `system_probe_config.enabled` is then **derived** — set to true if any module is enabled — and written back into the config, which is what the core agent reads to decide whether to talk to system-probe at all.

| Module | Platforms | Enabled when |
|---|---|---|
| `network_tracer` | Linux, Windows, macOS | `network_config.enabled` or `service_monitoring_config.enabled` or `ccm_network_config.enabled` or `infrastructure_mode: end_user_device` or `discovery.service_map.enabled` or (CWS enabled and `runtime_security_config.network_monitoring.enabled`) |
| `event_monitor` | Linux, Windows | `runtime_security_config.enabled` or `runtime_security_config.fim_enabled` or `sbom.enrichment.usage.enabled` (datadog.yaml) or (USM and `service_monitoring_config.enable_event_stream`) or (network tracer and `event_monitoring_config.network_process.enabled`) or GPU or Dynamic Instrumentation |
| `tcp_queue_length_tracer` | Linux | `system_probe_config.enable_tcp_queue_length` |
| `oom_kill_probe` | Linux | `system_probe_config.enable_oom_kill` |
| `ebpf` | Linux | `ebpf_check.enabled` |
| `noisy_neighbor` | Linux | `noisy_neighbor.enabled` |
| `process` | Linux | `system_probe_config.process_config.enabled` |
| `language_detection` | Linux | `system_probe_config.language_detection.enabled` |
| `compliance` | Linux | (`compliance_config.enabled` and `compliance_config.run_in_system_probe`) or `compliance_config.database_benchmarks.enabled` or legacy CWS compliance |
| `dynamic_instrumentation` | Linux | `dynamic_instrumentation.enabled` |
| `discovery` | Linux | `discovery.enabled` |
| `gpu` | Linux (`nvml` build tag) | `gpu_monitoring.enabled` |
| `ping` | Linux | `ping.enabled` |
| `traceroute` | Linux, Windows, macOS | `traceroute.enabled` |
| `privileged_logs` | Linux | `privileged_logs.enabled` |
| `windows_crash_detection` | Windows | `windows_crash_detection.enabled`; auto-enabled whenever the network tracer or event monitor is on, so the core agent can detect system-probe's own driver crashes |
| `software_inventory` | Windows, macOS | `software_inventory.enabled` |
| `injector` | Windows | `injector.enable_telemetry`; defaults to on when any other module is enabled |
| `logon_duration` | macOS | `logon_duration.enabled` |

What the smaller modules do, in one line each: `process` is a privileged `/proc` reader serving per-PID stats the process-agent cannot read itself (`POST /process/stats`); `language_detection` inspects binaries for language detection; the `ebpf`, `oom_kill_probe`, `tcp_queue_length_tracer`, and `noisy_neighbor` modules expose a `/check` endpoint returning `GetAndFlush()` JSON consumed by [Go core checks](../checks/corechecks.md) in the core agent (registered in [`corechecks_sysprobe.go`](<<<SRC>>>/pkg/commonchecks/corechecks_sysprobe.go)); `traceroute` and `ping` serve the [network path collector and NDM](../pipelines/ndm.md); `discovery` powers service discovery, partly implemented in Rust; `gpu` tracks CUDA kernel launches via uprobes on libcudart plus NVML queries; `dynamic_instrumentation` attaches DWARF-guided uprobes to Go binaries for probes delivered over [Remote Config](../configuration/remote-config.md).

## Configuration

system-probe reads `/etc/datadog-agent/system-probe.yaml` (Windows: `%ProgramData%\Datadog\system-probe.yaml`) with the same env-binding machinery as `datadog.yaml` (see [the configuration system](../configuration/overview.md)); the env override pattern is the standard `DD_` prefix plus a few bespoke aliases (`DD_SYSPROBE_SOCKET`, `DD_ENABLE_RUNTIME_COMPILER`, `DD_ENABLE_CO_RE`). Fleet policies are merged from `fleet_policies_dir/system-probe.yaml` ([`config.go`](<<<SRC>>>/pkg/system-probe/config/config.go)). Defaults live in [`pkg/config/setup/system_probe.go`](<<<SRC>>>/pkg/config/setup/system_probe.go) (plus [`system_probe_cws.go`](<<<SRC>>>/pkg/config/setup/system_probe_cws.go)).

Before the enablement matrix is computed, [`Adjust()`](<<<SRC>>>/pkg/system-probe/config/adjust.go) mutates the raw config exactly once (guarded by `system_probe_config.adjusted`): it migrates deprecated keys, validates the socket path, and runs per-area normalization in [`adjust_npm.go`](<<<SRC>>>/pkg/system-probe/config/adjust_npm.go), [`adjust_usm.go`](<<<SRC>>>/pkg/system-probe/config/adjust_usm.go), [`adjust_discovery.go`](<<<SRC>>>/pkg/system-probe/config/adjust_discovery.go), and [`adjust_security.go`](<<<SRC>>>/pkg/system-probe/config/adjust_security.go). Two adjustments regularly surprise people:

- Setting `system_probe_config.enabled: true` without configuring `network_config.enabled` (and with USM disabled) **implies `network_config.enabled: true`** — a legacy inference from the days when system-probe was only the network tracer.
- Enabling USM **flips `enable_runtime_compiler` and `enable_kernel_header_download` defaults to true**, which can trigger apt/yum/zypper metadata fetches on hosts where kernel headers are missing.

Core settings worth knowing: `system_probe_config.sysprobe_socket` (the module API address), `system_probe_config.external` (true means "someone else runs system-probe" — this process exits immediately), `log_file` (default `system-probe.log` next to the agent logs), `system_probe_config.debug_port` and `health_port` (both default 0 = off), `system_probe_config.telemetry_enabled`, and `system_probe_config.max_conns_per_message` (600, capped at 1000). On the *client* side, `datadog.yaml` has `check_system_probe_startup_time` and `check_system_probe_timeout` governing the grace period described below.

## eBPF loading strategies

All Linux eBPF modules load programs through [`pkg/ebpf`](<<<SRC>>>/pkg/ebpf), which supports three artifact families shipped under `${install_path}/embedded/share/system-probe/ebpf` (`system_probe_config.bpf_dir`): CO-RE object files under `co-re/`, C source bundles for runtime compilation, and legacy prebuilt object files. On amd64/arm64 the assets may also be embedded in the binary ([`bytecode`](<<<SRC>>>/pkg/ebpf/bytecode)).

### CO-RE

CO-RE (compile once, run everywhere) is the default (`system_probe_config.enable_co_re: true`). [`co_re.go`](<<<SRC>>>/pkg/ebpf/co_re.go) loads the object file and hands kernel type information (BTF) to cilium/ebpf for relocation. The [`orderedBTFLoader`](<<<SRC>>>/pkg/ebpf/btf.go) tries BTF sources in order:

1. A user-provided file at `system_probe_config.btf_path`.
1. The kernel's own BTF at `/sys/kernel/btf/vmlinux` (present on most kernels ≥5.4 with `CONFIG_DEBUG_INFO_BTF`).
1. The **embedded collection**: per-distro minimized BTF shipped as `minimized-btfs.tar.xz` in `bpf_dir`, extracted into `system_probe_config.btf_output_dir` (default `/var/tmp/datadog-agent/system-probe/btf`).
1. A **remote-config download** (`system_probe_config.remote_config_btf_enabled`, default true) fetching BTF for the exact kernel from `install.datadoghq.com`.

Loaded BTF is cached and flushed roughly a minute after last use (and explicitly after all modules finish loading), so a memory spike at startup is expected. `/debug/ebpf_btf_loader_info` reports which source won, and the `ebpf__core_load_success` telemetry carries a `btf_type` tag (`ebpf__core_load_error` carries an `error_type` tag instead).

### Runtime compilation

When enabled (`system_probe_config.enable_runtime_compiler`; default false, but auto-defaulted to true when USM is enabled), [`pkg/ebpf/bytecode/runtime`](<<<SRC>>>/pkg/ebpf/bytecode/runtime) compiles the shipped C sources with an embedded clang ([`pkg/ebpf/compiler`](<<<SRC>>>/pkg/ebpf/compiler)) against kernel headers found in `system_probe_config.kernel_header_dirs` or downloaded via the host's package repos (`enable_kernel_header_download`). Output goes to `system_probe_config.runtime_compiler_output_dir` (default `/var/tmp/datadog-agent/system-probe/build`), keyed by a hash of inputs and flags so recompilation is skipped across restarts.

### Prebuilt

Prebuilt objects are the legacy path: compiled ahead of time without BTF, relying on **offset guessing** at startup (brute-forcing kernel struct offsets by creating known connections — see the [network page](network-monitoring.md)). [`prebuilt/deprecate.go`](<<<SRC>>>/pkg/ebpf/prebuilt/deprecate.go) deprecates prebuilt on kernels ≥6.0 (RHEL: ≥5.14); on such hosts `allow_prebuilt_fallback` auto-defaults to false.

### The fallback chain

Loaders that support multiple modes (the NPM kprobe tracer in [`kprobe/tracer.go`](<<<SRC>>>/pkg/network/tracer/connection/kprobe/tracer.go), the eBPF conntracker, several check probes) follow the same pattern:

```text
CO-RE ----load error (non-verifier)----> runtime compilation ----error----> prebuilt
  |                                        (if enable_runtime_compiler        (if allow_prebuilt_fallback)
  |                                         && allow_runtime_compiled_fallback)
  +--verifier error--> abort (no fallback: the verifier would reject the
                       other flavors for the same reason)
```

Support infrastructure in `pkg/ebpf` used by all modules: a `Manager` wrapper whose modifiers patch every program with error-reporting instrumentation, `maps.GenericMap` and `MapCleaner` (periodic TTL eviction), perf/ring-buffer event handlers with usage telemetry, the shared [`uprobes`](<<<SRC>>>/pkg/ebpf/uprobes) attacher (used by USM TLS and GPU), and map/program name mappings ([`names`](<<<SRC>>>/pkg/ebpf/names)) consumed by the `ebpf` check. The eBPF C sources live in [`pkg/ebpf/c`](<<<SRC>>>/pkg/ebpf/c), [`pkg/network/ebpf/c`](<<<SRC>>>/pkg/network/ebpf/c), and [`pkg/security/ebpf/c`](<<<SRC>>>/pkg/security/ebpf/c) and are built with Bazel.

## IPC, ports, and the security model

| Channel | Details |
|---|---|
| Module HTTP API | UDS at `${run_path}/sysprobe.sock` on Linux/macOS ([`listener_unix.go`](<<<SRC>>>/pkg/system-probe/api/server/listener_unix.go): socket chmod `0720`, owner-restricted to the agent user); named pipe `\\.\pipe\dd_system_probe` on Windows ([`listener_windows.go`](<<<SRC>>>/pkg/system-probe/api/server/listener_windows.go): DACL granting full access to Administrators/SYSTEM and read/write to `ddagentuser`) |
| Debug server | `127.0.0.1:<system_probe_config.debug_port>` (expvar, pprof, telemetry) — off by default; separate health probe port `system_probe_config.health_port`, also off by default |
| CWS to security-agent | gRPC over `runtime_security_config.socket` (default `/opt/datadog-agent/run/runtime-security.sock`; Windows `localhost:3335`), served by [`pkg/security/module/server.go`](<<<SRC>>>/pkg/security/module/server.go) — *not* the sysprobe HTTP socket |
| system-probe to core agent | Remote tagger, remote workloadmeta, remote hostname, configsync, runtime settings — all over the core agent's gRPC IPC (default `localhost:5001`) authenticated with the shared auth token; Dynamic Instrumentation opens its own secure gRPC client for Remote Config subscriptions |
| system-probe to intake | Only with direct send: the connections forwarder and [event platform forwarder](../pipelines/event-platform.md) send network payloads straight from system-probe; internal telemetry goes to statsd on `localhost:8125` |
| Windows drivers | Device objects `\\.\ddnpm` and ddprocmon, IOCTLs issued by system-probe only |

/// warning
The module API carries **no authentication token**. Everything except `POST /agent-restart` is protected only by the socket file mode (Linux/macOS) or the pipe DACL (Windows). Anything that can open the socket can read connection data, trigger traceroutes and pings, and dump eBPF maps. On Kubernetes, the hostPath/emptyDir volume carrying `sysprobe.sock` must not be mounted into workload containers.
///

The client library ([`pkg/system-probe/api/client`](<<<SRC>>>/pkg/system-probe/api/client)) fakes URLs as `http://sysprobe/<module>/<endpoint>` over a custom dialer. A singleton startup checker probes `/debug/stats` and returns `ErrNotStartedYet` during the `check_system_probe_startup_time` window, so checks in the core agent log "system-probe not started yet" warnings instead of errors while system-probe warms up — expected noise right after start, not a fault.

## Privileges and deployment modes

- **Linux host**: runs as root via the systemd unit described above. Runtime-compilation artifacts and extracted BTF live under `/var/tmp/datadog-agent/system-probe/`.
- **Docker / Kubernetes**: system-probe runs as a separate container in the agent pod, either `privileged` or with a capability set along the lines of `CAP_SYS_ADMIN` (or fine-grained `CAP_BPF` + `CAP_PERFMON` + `CAP_SYS_RESOURCE` + `CAP_SYS_PTRACE` + `CAP_NET_ADMIN` + `CAP_NET_RAW` + `CAP_IPC_LOCK` + `CAP_CHOWN`), with hostPID, mounts of `/sys/kernel/debug` and the host `/proc` (pointed to by `HOST_PROC`), and a shared volume for `sysprobe.sock` so the agent and process-agent containers can dial it — `DD_SYSPROBE_SOCKET` points both sides at the shared path. See [Runtime environments](../deployment/environments.md).
- **ECS Fargate**: no eBPF is permitted, so NPM falls back to the packet-capture ("ebpfless") tracer and CWS runs in its eBPF-less ptrace mode; an empty hostname is tolerated only in the Fargate sidecar case.
- **Windows**: the `datadog-system-probe` service runs as LocalSystem. Network tracing and USM come from the closed-source `ddnpm` kernel driver (device `\\.\ddnpm`, interfaced through [`pkg/network/driver`](<<<SRC>>>/pkg/network/driver)) with ETW supplying TLS visibility; CWS process monitoring uses the second driver, `ddprocmon`. The MSI installs the drivers; if `ddnpm` is missing, tracer creation fails with a "reinstall with NPM enabled" error.
- **macOS**: a much smaller module set — the ebpfless network tracer over libpcap, traceroute, software inventory, and logon duration.
- The Cluster Agent has no system-probe: everything here is node-level.

## Operating and debugging

Global endpoints on the module socket (reachable with `curl --unix-socket /opt/datadog-agent/run/sysprobe.sock http://sysprobe/...`):

| Endpoint | Purpose |
|---|---|
| `/debug/stats` | Per-module stats and per-module startup `Error` entries; also the client readiness probe |
| `POST /module-restart/{module_name}` | Recreate a module in place |
| `GET /debug/pprof/` | Go pprof profiles |
| `/debug/vars` | expvar (includes the `modules` map) |
| `/telemetry` | Prometheus-format internal telemetry, including the eBPF collectors |
| `/debug/ebpf_btf_loader_info` | Which BTF source the loader used (Linux) |
| `/debug/dmesg`, `/debug/selinux_sestatus`, `/debug/selinux_semodule_list` | Kernel and SELinux context for flares (Linux) |
| `POST /agent-restart` | Auth-token-protected restart hook |

Each module adds its own endpoints under `/{module_name}/` — the network tracer's extensive debug surface is cataloged on the [network monitoring page](network-monitoring.md). The binary also has diagnostic subcommands: `system-probe config`, `system-probe debug <endpoint>`, `system-probe modrestart <module>`, and `system-probe ebpf` for BTF inspection. Status for NPM/USM appears in `agent status` on the core agent side (see [Status, health, and telemetry](../operations/introspection.md)), and [flares](../operations/flare.md) collect the debug endpoints automatically.

Common failure modes to check first: a kernel below a feature's gate (see the [kernel gates table](network-monitoring.md#kernel-version-gates)), missing tracefs (`sys-kernel-debug.mount`), conntrack initialization failure (netlink permissions; `network_config.ignore_conntrack_init_failure` exists as an escape hatch), missing kernel headers when runtime compilation is the selected strategy, and on Windows a missing or stopped `ddnpm` driver service.

## Gotchas

- **`system_probe_config.enabled: true` alone enables NPM** via the legacy inference in `Adjust()` — it is not just a master switch.
- **Module init order is encoded in a slice** (`moduleOrder`), and the GPU-to-event-monitor dependency travels through a package-global variable. Adding a module with dependencies means editing the order deliberately.
- **A failed module does not fail the process.** If NPM silently produces no data, check `/debug/stats` for a per-module `Error` before reading eBPF internals.
- **CO-RE verifier errors never fall back** to runtime compilation or prebuilt; other CO-RE load errors do, per the `allow_*_fallback` flags.
- **Enabling USM silently enables the runtime compiler and kernel header download** as defaults, which can start package-manager metadata fetches.
- **`system_probe_config.external: true` makes the binary exit immediately** — confusing when set by accident.
- **The `ErrNotEnabled` exit sleeps five seconds on purpose** to avoid supervisor crash-loop back-off; a fast-exiting system-probe with no error in the log usually means no module is enabled.
- **`system-probe-lite` exec**: with `discovery.use_system_probe_lite` and only discovery enabled, the process execs into a different binary — the PID is unchanged but you are no longer debugging `system-probe`.
- **rlimit memlock is removed process-wide** in `ebpf.Setup()`; required for map allocation on kernels before 5.11.
- **On Windows, 20 minutes without a `/connections` request makes system-probe exit** (with a critical log and an Event Viewer entry) so the service manager restarts it; a misconfigured process-agent thus looks like recurring system-probe restarts.
