# Binaries and flavors

-----

The Datadog Agent is not one program but a family of cooperating processes built from a set of Go (plus two Rust) binaries whose entry points live under [`cmd/`](<<<SRC>>>/cmd). On a typical Linux host you will find the core `agent`, `trace-agent`, `process-agent`, `system-probe`, `security-agent`, and the fleet `installer` running side by side, all talking to each other over the channels described in [Inter-process communication](ipc.md). This page inventories every shipped binary, explains the multi-call dispatch of the main `agent` binary, describes build-time flavors, and maps binaries to the deployment modes that run them. Who *starts* these processes on each platform is covered in [Process supervision](supervision.md); how each one boots internally is covered in [Startup and lifecycle](lifecycle.md).

## The multi-call `agent` binary

The main `agent` binary is a multi-call binary, in the style of BusyBox. [`cmd/agent/main.go`](<<<SRC>>>/cmd/agent/main.go) keeps a registry of "personalities" (`registerAgent`) and selects one at startup:

1. If the `DD_BUNDLED_AGENT` environment variable is set, its value picks the personality.
1. Otherwise the basename of `argv[0]` is used (with the extension stripped), so renaming or symlinking the binary changes what it becomes.
1. The names `agent`, `datadog-agent`, and `dd-agent` map to the core agent. On Linux builds with the `bundle_installer` build tag, [`cmd/agent/installer.go`](<<<SRC>>>/cmd/agent/installer.go) registers `installer` and `datadog-installer` as personalities, which is how the fleet installer is bundled inside the agent binary on Linux.
1. An unrecognized name falls back to the core agent with only a note on stderr: `Invoked as '<name>', acting as main Agent.`

/// warning
Because the fallback is silent apart from one stderr line, copying the agent binary to a differently-named wrapper (or leaking `DD_BUNDLED_AGENT` into a child environment) can change which program actually runs. If a process behaves like the wrong agent, check `argv[0]` and `DD_BUNDLED_AGENT` first.
///

On Windows, [`cmd/agent/main_windows.go`](<<<SRC>>>/cmd/agent/main_windows.go) additionally detects whether the process was launched by the Service Control Manager and enters `servicemain.Run` for the `datadogagent` service instead of the CLI path.

## Binary inventory

All entry points are under `cmd/`; each binary is assembled from components as described in the [component overview](../components/overview.md).

| Binary | Entry point | What it does | Where it runs |
|---|---|---|---|
| `agent` | [`cmd/agent`](<<<SRC>>>/cmd/agent) | Core agent: checks, DogStatsD, logs, metadata, API hub | Everywhere |
| `trace-agent` | [`cmd/trace-agent`](<<<SRC>>>/cmd/trace-agent) | APM trace intake and processing | Host, containers |
| `trace-loader` | [`cmd/loader`](<<<SRC>>>/cmd/loader) | Socket-activation shim that spawns trace-agent on first connection | Linux packaged installs |
| `process-agent` | [`cmd/process-agent`](<<<SRC>>>/cmd/process-agent) | Process and container collection (see caveat below) | Windows, macOS, containers |
| `system-probe` | [`cmd/system-probe`](<<<SRC>>>/cmd/system-probe) | eBPF/kernel-level modules (NPM, USM, CWS events, GPU, ...) | Linux (root), Windows (LocalSystem) |
| `security-agent` | [`cmd/security-agent`](<<<SRC>>>/cmd/security-agent) | Compliance (CSPM) and CWS userland side | Host, containers |
| `dogstatsd` | [`cmd/dogstatsd`](<<<SRC>>>/cmd/dogstatsd) | Standalone StatsD-only server | Own package/image |
| `datadog-cluster-agent` | [`cmd/cluster-agent`](<<<SRC>>>/cmd/cluster-agent) | Kubernetes cluster-level agent (DCA) | Kubernetes only |
| `datadog-cluster-agent-cloudfoundry` | [`cmd/cluster-agent-cloudfoundry`](<<<SRC>>>/cmd/cluster-agent-cloudfoundry) | Cloud Foundry variant of the DCA | Cloud Foundry |
| `installer` | [`cmd/installer`](<<<SRC>>>/cmd/installer) | Fleet Automation installer: CLI, daemon, package maintainer logic | Host installs |
| `otel-agent` | [`cmd/otel-agent`](<<<SRC>>>/cmd/otel-agent) | DDOT: Datadog distribution of the OpenTelemetry Collector | Host (add-on package), containers |
| `serverless-init` | [`cmd/serverless-init`](<<<SRC>>>/cmd/serverless-init) | All-in-one wrapper for PaaS container runtimes | Cloud Run, ACA, App Runner, ... |
| `iot-agent` | [`cmd/iot-agent`](<<<SRC>>>/cmd/iot-agent) | Stripped-down core agent (no Python) | IoT devices |
| `agent-data-plane` | (external repo) | ADP: Rust reimplementation of the DogStatsD pipeline (Saluki) | Host, opt-in |
| `dd-procmgrd` | [`pkg/procmgr/rust`](<<<SRC>>>/pkg/procmgr/rust) | Rust process-manager daemon (emerging subagent supervisor) | Host, opt-in |
| `ddtray` | [`cmd/systray`](<<<SRC>>>/cmd/systray) | Windows tray icon UI | Windows |
| `cws-instrumentation` | [`cmd/cws-instrumentation`](<<<SRC>>>/cmd/cws-instrumentation) | Helper injected into workload containers for CWS ptrace instrumentation | Kubernetes workloads |
| `host-profiler` | [`cmd/host-profiler`](<<<SRC>>>/cmd/host-profiler) | Standalone eBPF full-host profiler on OTel collector architecture | Linux |
| `privateactionrunner` | [`cmd/privateactionrunner`](<<<SRC>>>/cmd/privateactionrunner) | Executes Datadog private actions | Host, containers |
| `secret-generic-connector` | [`cmd/secret-generic-connector`](<<<SRC>>>/cmd/secret-generic-connector) | Embedded secrets-backend helper, exec'd as a short-lived subprocess | All agent processes |
| `secrethelper` | [`cmd/secrethelper`](<<<SRC>>>/cmd/secrethelper) | Reads secrets from files/Kubernetes secrets for `secret_backend_command` | Containers |
| `sbomgen` | [`cmd/sbomgen`](<<<SRC>>>/cmd/sbomgen) | SBOM generation via Trivy | Container scanning |
| `ai_prompt_logger` | [`cmd/ai_prompt_logger`](<<<SRC>>>/cmd/ai_prompt_logger) | Rust Chrome Native Messaging host + desktop monitor for AI-usage tracking | Windows, macOS end-user devices |
| `config-stream-client` | [`cmd/config-stream-client`](<<<SRC>>>/cmd/config-stream-client) | Test client for the `AgentSecure.StreamConfigEvents` gRPC API | Development only |

### Core `agent`

`agent run` hosts the check scheduler and collector (Python checks in-process via rtloader CGo — see [Python checks](../checks/python.md)), the DogStatsD server (UDP 8125 + UDS — see [DogStatsD internals](../dogstatsd/internals.md)), the logs-agent, OTLP ingest when configured ([OTLP ingest](../otel/otlp-ingest.md)), the remote-config service, and the API servers that make it the hub every other process connects back to. It spawns [JMXFetch](../checks/jmx.md) as a `java` subprocess ([`pkg/jmxfetch/jmxfetch.go`](<<<SRC>>>/pkg/jmxfetch/jmxfetch.go)), passing it `--ipc_host`/`--ipc_port` so it can fetch check configs from the agent API and report metrics through DogStatsD.

On Linux, the process and container checks that historically lived in process-agent now run inside the core agent unconditionally: [`pkg/process/util/coreagent/coreagent_linux.go`](<<<SRC>>>/pkg/process/util/coreagent/coreagent_linux.go) hardcodes `ProcessChecksRunInCoreAgent()` to `true`, while the `_other.go` variant returns `false` so Windows and macOS still use process-agent. See the [process pipeline](../pipelines/processes.md).

Besides `run`, the binary carries roughly 40 CLI subcommands under [`cmd/agent/subcommands`](<<<SRC>>>/cmd/agent/subcommands) (`status`, `check`, `flare`, `diagnose`, `config`, `hostname`, `taggerlist`, `workloadlist`, `streamlogs`, `integrations`, `controlsvc` on Windows, ...). Nearly all of them are thin HTTPS clients of the running agent's `cmd_port` API, authenticated with the IPC auth token — see [Diagnostics and CLI tools](../operations/diagnostics.md).

### `trace-agent` and `trace-loader`

The trace-agent receives traces on TCP `apm_config.receiver_port` (8126), a UDS receiver socket, and optionally a Windows named pipe — see the [trace pipeline](../pipelines/traces.md). On Linux packaged installs it is not exec'd directly: the systemd unit runs `trace-loader` ([`cmd/loader/main_nix.go`](<<<SRC>>>/cmd/loader/main_nix.go), installed at `embedded/bin/trace-loader`), which loads `datadog.yaml`, exits immediately if APM is disabled, and — when `apm_config.socket_activation.enabled` is set — opens the receiver sockets itself and only execs the real trace-agent when a first client connects. It can even absorb TCP liveness probes without triggering a spawn (`apm_config.socket_activation.handle_tcp_probe`). If socket activation is disabled or anything goes wrong, it execs the trace-agent directly. The Heroku flavor replaces trace-loader with a noop shim ([`cmd/loader/BUILD.bazel`](<<<SRC>>>/cmd/loader/BUILD.bazel)).

The trace-agent also proxies remote-config requests from tracer libraries (`/v0.7/config` on 8126) to the core agent's gRPC API, and uses the optional remote tagger so it degrades gracefully when no core agent is present.

### `process-agent`

On Linux its flagship checks run in the core agent (see above), and a process-agent container deployed anyway (for example by an older Helm chart) detects this and idles instead of crash-looping (`shouldStayAlive` in `cmd/process-agent/command/main_common.go`). It still does real work on Windows and macOS, and wherever checks need [system-probe](../ebpf/system-probe.md) data. On Windows the core agent's workloadmeta pulls process entities from it over a gRPC stream on port 6262.

### `system-probe`

Runs as root on Linux and LocalSystem on Windows, hosting pluggable modules under [`cmd/system-probe/modules`](<<<SRC>>>/cmd/system-probe/modules) (network tracer, USM, event monitor for CWS, OOM-kill and TCP-queue probes, GPU, traceroute, discovery, dynamic instrumentation, ...). It serves plain HTTP over a permission-guarded UDS or named pipe rather than the token-authenticated HTTPS the other processes use. Details in [system-probe](../ebpf/system-probe.md); the socket specifics are in [Inter-process communication](ipc.md).

### `security-agent`

The userland half of [Workload Protection](../ebpf/cws.md) and [Compliance](../ebpf/compliance.md). It consumes CWS events from system-probe's event-monitor gRPC socket, and runs as root on Linux (its systemd unit sets no `User=`) but as the regular agent user on Windows. Its systemd unit only starts when a `security-agent.yaml` exists (see [Process supervision](supervision.md)).

### `dogstatsd`

A slim StatsD-only build with its own flavor, its own package `datadog-dogstatsd` ([`omnibus/config/software/datadog-dogstatsd.rb`](<<<SRC>>>/omnibus/config/software/datadog-dogstatsd.rb)), its own Docker image (`datadog/dogstatsd`), and even its own Windows MSI. It shares the DogStatsD server components with the core agent (see [DogStatsD internals](../dogstatsd/internals.md)).

### `datadog-cluster-agent` (DCA)

Kubernetes-only, deployed as its own container image ([`Dockerfiles/cluster-agent`](<<<SRC>>>/Dockerfiles/cluster-agent)) and built with the `kubeapiserver` build tag. It serves cluster-level tagging, cluster checks dispatch, external metrics, and admission webhooks to node agents over a muxed HTTPS+gRPC listener on port 5005 with a two-token auth model. See [Cluster Agent](../containers/cluster-agent.md) for the full story; the port and token summary lives in [Inter-process communication](ipc.md).

### `installer`

One binary, three roles: a CLI (`installer install/remove/postinst ...`) invoked by package maintainer scripts, a daemon (`installer run`) that executes remote-config-driven package upgrades using the experiment mechanism, and a bundled personality of the agent multi-call binary. The daemon and the `/opt/datadog-packages` layout are covered in [Fleet automation and the installer](../deployment/fleet.md); how its units are wired is in [Process supervision](supervision.md).

### `otel-agent` (DDOT)

The Datadog distribution of the OpenTelemetry Collector, built with converged Agent components (tagger, forwarder). Shipped as a separate `datadog-agent-ddot` package that extends an existing agent install under `/opt/datadog-agent/ext/ddot/`, as its own container image ([`Dockerfiles/agent-ddot`](<<<SRC>>>/Dockerfiles/agent-ddot)), and as a bundled service in the all-in-one agent image. It runs `otel-agent run --config otel-config.yaml --core-config datadog.yaml` as the agent user, exposes its `ddflareextension` on `localhost:7777` for flare integration, and registers with the core agent's remote-agent registry. See [DDOT collector](../otel/ddot.md).

### `serverless-init`

A single process embedding DogStatsD, trace intake, and logs for PaaS container platforms (Cloud Run, Azure Container Apps, AWS App Runner, Azure Spring Apps). [`cmd/serverless-init/mode/mode.go`](<<<SRC>>>/cmd/serverless-init/mode/mode.go) implements two modes: **init mode**, where the binary is the container `ENTRYPOINT` and wraps the user command as a child process, and **sidecar mode**, where it runs as a separate sidecar container. It has no IPC servers and no remote tagger. The AWS Lambda extension is a separate build from `pkg/serverless/` in the `datadog-lambda-extension` repository.

### `agent-data-plane` (ADP) and `dd-procmgrd`

ADP is a Rust reimplementation of the DogStatsD pipeline (project Saluki) that is **not built from this repository**: omnibus downloads a prebuilt tarball at package build time ([`omnibus/config/software/datadog-agent-data-plane.rb`](<<<SRC>>>/omnibus/config/software/datadog-agent-data-plane.rb)), so repo code and the shipped ADP version can drift. It registers with the core agent as a remote agent for status and flare integration.

`dd-procmgrd` is a Rust process-manager daemon whose source *is* in-repo ([`pkg/procmgr/rust`](<<<SRC>>>/pkg/procmgr/rust)) with a Go gRPC client in [`pkg/procmgr/coat`](<<<SRC>>>/pkg/procmgr/coat). It supervises processes declared in `processes.d/*.yaml` and is the emerging replacement for one-systemd-unit-per-subagent; DDOT is the first migrated service. See [Process supervision](supervision.md).

## Flavors

There are two distinct notions of "flavor" that are easy to conflate:

**Build flavors** (`AgentFlavor` in [`tasks/flavor.py`](<<<SRC>>>/tasks/flavor.py)) select what gets built and packaged: `base`, `iot`, `heroku`, `dogstatsd`, and `fips`. The FIPS flavor builds the same binaries with the `goexperiment.systemcrypto` and `requirefips` build tags (see `FIPS_TAGS` in [`tasks/build_tags.bzl`](<<<SRC>>>/tasks/build_tags.bzl)), routing Go cryptography through the platform's FIPS-validated module. Heroku is the full agent with packaging tweaks for the buildpack environment (for example, the noop trace-loader). IoT is a genuinely different binary ([`cmd/iot-agent`](<<<SRC>>>/cmd/iot-agent), package definition in [`omnibus/config/projects/iot-agent.rb`](<<<SRC>>>/omnibus/config/projects/iot-agent.rb)) with no Python/rtloader and a minimal check set.

**Runtime flavors** ([`pkg/util/flavor/flavor.go`](<<<SRC>>>/pkg/util/flavor/flavor.go)) identify which process is running so shared code can adapt: `agent`, `iot_agent`, `cluster_agent`, `dogstatsd`, `security_agent`, `serverless_agent`, `heroku_agent`, `process_agent`, `trace_agent`, `otel_agent`, `system_probe`, `host_profiler`, `private_action_runner`. Each binary calls `flavor.SetFlavor` at startup; setting the IoT flavor forces the `iot_host` config key to `true`, and `GetFlavor()` reports `iot_agent` whenever `iot_host` is set even on a non-IoT binary.

DDOT and serverless-init are not build flavors — they are separate binaries and packages. Beyond flavors, Go build tags (`python`, `otlp`, `kubeapiserver`, `docker`, ...) control feature inclusion per binary; the source of truth is [`tasks/build_tags.bzl`](<<<SRC>>>/tasks/build_tags.bzl), and this is why the repository rule is to always build through `dda inv`, never raw `go build`.

## Which deployment modes run which binaries

| Binary | Linux host | Windows host | macOS host | Single container | Kubernetes DaemonSet | Fargate sidecar |
|---|---|---|---|---|---|---|
| `agent` | yes (systemd) | yes (service) | yes (launchd) | yes (s6) | yes (own container) | yes |
| `trace-agent` | yes (via trace-loader) | yes | shipped, no service | yes (s6) | yes (own container) | yes |
| `process-agent` | mostly idle (checks in core agent) | yes | shipped, no service | yes (s6) | optional container | optional |
| `system-probe` | yes (root) | yes (LocalSystem + drivers) | launchd daemon | privileged only | yes (privileged container) | no |
| `security-agent` | gated on security-agent.yaml | yes | no | yes (s6) | optional container | optional (CWS lite) |
| `installer` (daemon) | yes (root) | demand service (`remote_updates`) | no | no | no | no |
| `otel-agent` | optional add-on package | no | no | optional | optional container | no |
| `cluster-agent` | no | no | no | no | own Deployment | no |
| `agent-data-plane` | optional | optional (dd-procmgr) | launchd daemon | optional (s6) | no | no |
| `dd-procmgrd` | optional | optional service | no | no | no | no |
| `serverless-init` | no | no | no | PaaS runtimes only | no | no |

On macOS there are no separate trace-agent or process-agent launchd services; on Fargate there is no system-probe because there is no host kernel access. Details of the supervision trees behind this table are in [Process supervision](supervision.md); packaging is covered in [Packaging](../deployment/packaging.md) and runtime environment differences in [Runtime environments](../deployment/environments.md).

## File layout on a Linux host install

1. `/opt/datadog-agent/bin/agent/agent` — the multi-call core agent binary.
1. `/opt/datadog-agent/embedded/bin/` — everything else: `trace-agent`, `process-agent`, `security-agent`, `system-probe`, `installer`, `trace-loader`, `agent-data-plane`, `dd-procmgrd`, `privateactionrunner`, `secret-generic-connector`.
1. `/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent` — the DDOT add-on payload.
1. `/opt/datadog-packages/datadog-agent/{stable,experiment}` — symlinks mapping the package install into the fleet layout ([`packages/agent/linux/BUILD.bazel`](<<<SRC>>>/packages/agent/linux/BUILD.bazel)); see [Fleet automation](../deployment/fleet.md).

## Gotchas

1. **Renaming the agent binary changes what it is.** The multi-call dispatch means `argv[0]` is semantically load-bearing; an unknown name silently falls back to the core agent.
1. **`ps` may not show a trace-agent.** On Linux package installs with socket activation, `trace-loader` holds the sockets until the first client connects; seeing `trace-loader` instead of `trace-agent` in the process list is normal.
1. **Process checks moved into the core agent on Linux only.** Debugging live-process data on Linux means looking at the core agent; on Windows/macOS it is still process-agent.
1. **ADP is versioned outside this repository** and injected at package build time via environment variables — code you read under a given commit may not match the shipped `agent-data-plane`.
1. **The IoT flavor is partly a runtime property**: any agent with `iot_host: true` reports the `iot_agent` flavor, not just the dedicated binary.
