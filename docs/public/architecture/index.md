# Datadog Agent Architecture

-----

This site describes how the Datadog Agent works internally: the processes it runs, the frameworks it is built on, and the paths data takes from a monitored host to the Datadog intake. It is written for engineers who know Go but are new to a given subsystem — each page names the real packages, types, and functions involved, so you can go from a concept straight to the code.

## What the Agent is

The Datadog Agent is the software that runs on customer infrastructure — hosts, VMs, containers, Kubernetes nodes, serverless environments — and collects metrics, traces, logs, processes, network activity, and security events, then forwards them to the Datadog platform. It is written primarily in Go (with a handful of Rust binaries and an embedded Python interpreter for integration checks), built from this repository for Agent versions 6 and 7, and ships on Linux, Windows, and macOS as OS packages, container images, and OCI fleet packages.

"The Agent" is really a family of cooperating processes built from binaries under [`cmd/`](<<<SRC>>>/cmd). A typical Linux host install runs the core `agent`, `trace-agent`, `process-agent`, `system-probe`, `security-agent`, and the fleet `installer` daemon, with optional extras such as the `otel-agent` (DDOT). On Kubernetes, a `cluster-agent` Deployment additionally serves as the single point of contact with the Kubernetes API server for all node agents.

## The shape of the system

Three structural ideas explain most of the codebase:

1. **Multi-process, with the core agent as the hub.** Privileged collection (eBPF, kernel drivers) lives in `system-probe`; high-volume APM intake lives in `trace-agent`; the core agent owns the shared services everyone else needs. Satellite processes connect back to the core agent's authenticated HTTPS + gRPC API on `localhost:5001` (`cmd_port`) for tags (remote tagger), workload metadata, configuration sync, and remote config, using a shared `auth_token` and a self-signed TLS certificate created by the core agent on first run. See [Binaries and flavors](processes/binaries.md) and [Inter-process communication](processes/ipc.md).
1. **Component-based assembly.** Each binary is assembled from a shared catalog of ~160 components under [`comp/`](<<<SRC>>>/comp), wired together with Uber's Fx dependency-injection framework. A component is an interface in a `def/` package with Fx-free implementations in `impl*/` packages; which components a binary instantiates depends on what its entry point requires. Cross-cutting surfaces — the status page, flares, runtime settings, HTTP endpoints — are federated through Fx value groups, so any component can contribute to them. See the [component overview](components/overview.md) and the [Fx primer](components/fx.md).
1. **Pipelines that end at the forwarder.** Every product follows the same broad shape: an intake surface (DogStatsD listener, file tailer, check scheduler, trace receiver) feeds domain-specific processing (aggregation, decoding, obfuscation, sampling), which produces serialized payloads handed to a forwarder that owns retries, backoff, dual-shipping, and failover to the Datadog intake. See the [data pipelines overview](pipelines/overview.md) and [Forwarder and resilience](pipelines/forwarder.md).

Configuration is the cross-cutting fourth piece: a layered, schema-validated key/value store (defaults, file, env vars, fleet policies, secrets, remote config, CLI) shared by every process, with satellite processes mirroring the core agent's resolved values over IPC. See [the configuration system](configuration/overview.md).

## System diagram

```text
 data sources                        Agent processes (one host / node)              Datadog intake (per site)

 StatsD clients --- UDP 8125 / UDS ---+
 integrations (Go + Python checks) ---|    +------------------------------+
 log files / journald / sockets ------+--->|          core agent          |-- metrics ---> <version>-app.agent.<site>
 OTLP apps --- gRPC 4317 / HTTP 4318 -+    |  checks - DogStatsD - logs   |-- logs ------> agent-http-intake.logs.<site>
                                           |  OTLP ingest - remote config |-- events ----> <product>-intake.<site>
                                           +--------------+---------------+   (event platform tracks)
                                                          ^
                              IPC: HTTPS + gRPC on        |  remote tagger, workloadmeta,
                              localhost:5001              |  config sync, remote config
                              (auth token + TLS)          |
                    +------------------+------------------+
                    |                  |                  |
 tracer SDKs ---> trace-agent ---------)------ traces / stats -----------------> trace.agent.<site>
  (TCP 8126 / UDS)                     |
 processes / containers ---> process-agent --- process payloads ---------------> process.<site>
                                       |
 kernel events (eBPF / ETW) ---> system-probe --- UDS / named pipe ---> (serves core, process, security agents)
                                       |
                                       +--- CWS events (gRPC) ---> security-agent --> runtime-security / CSPM intake

 Kubernetes API server <--- cluster-agent (Deployment, port 5005) <--- node agents (cluster tags, cluster checks)
```

Every arrow on the right side goes through a forwarder: the [default forwarder](pipelines/forwarder.md) for metrics and metadata, the [event platform forwarder](pipelines/event-platform.md) for track-based products, and dedicated writers in the trace-agent and process-agent. All of them support `additional_endpoints` dual-shipping, proxies, and Multi-Region Failover.

## Processes at a glance

| Process | Entry point | Role | Privileges (Linux) |
|---|---|---|---|
| core agent | [`cmd/agent`](<<<SRC>>>/cmd/agent) | Checks, DogStatsD, logs, OTLP ingest, remote config, IPC hub (`cmd_port` 5001) | `dd-agent` |
| trace-agent | [`cmd/trace-agent`](<<<SRC>>>/cmd/trace-agent) | APM trace intake (8126), sampling, APM stats, tracer-facing proxies | `dd-agent` |
| process-agent | [`cmd/process-agent`](<<<SRC>>>/cmd/process-agent) | Live processes/containers; on Linux most of its checks now run inside the core agent | `dd-agent` |
| system-probe | [`cmd/system-probe`](<<<SRC>>>/cmd/system-probe) | eBPF/kernel collection modules (NPM, USM, CWS events, GPU, ...), serves other agents over a local socket | root |
| security-agent | [`cmd/security-agent`](<<<SRC>>>/cmd/security-agent) | Compliance benchmarks; forwards CWS events from system-probe | root |
| cluster-agent | [`cmd/cluster-agent`](<<<SRC>>>/cmd/cluster-agent) | Kubernetes-only Deployment (API port 5005): apiserver facade, cluster checks, admission controller, external metrics | in-cluster |
| installer | [`cmd/installer`](<<<SRC>>>/cmd/installer) | Fleet automation daemon: remote package installs/upgrades with rollback | root |
| otel-agent | [`cmd/otel-agent`](<<<SRC>>>/cmd/otel-agent) | DDOT: full OpenTelemetry Collector distribution reusing Agent pipelines | `dd-agent` |
| dogstatsd | [`cmd/dogstatsd`](<<<SRC>>>/cmd/dogstatsd) | Standalone slim StatsD-only server (own package and image) | `dd-agent` |

Which of these actually run — and who supervises them — depends on the platform and deployment mode; see [Binaries and flavors](processes/binaries.md) and [Process supervision](processes/supervision.md) for the full inventory, including the more specialized binaries (`serverless-init`, `cws-instrumentation`, `iot-agent`, and others).

## Documentation map

### Processes

How the binaries are laid out, started, and talk to each other. [Binaries and flavors](processes/binaries.md) inventories every entry point under `cmd/` (including the multi-call dispatch that bundles the installer inside the agent binary). [Startup and lifecycle](processes/lifecycle.md) follows a binary from `main()` through Fx app construction to a running daemon. [Inter-process communication](processes/ipc.md) covers the auth token, TLS artifacts, the `localhost:5001` API, and every local socket and port. [Process supervision](processes/supervision.md) explains systemd units, Windows services, launchd, and s6-overlay in containers.

### Components

The component framework that all binaries are assembled from: the [overview](components/overview.md) explains what a component is, [creating components](components/creating-components.md) and [using components](components/using-components.md) cover the def/impl/fx/mock layout, and the [Fx primer](components/fx.md) introduces the dependency-injection framework itself.

### Configuration

[The configuration system](configuration/overview.md) explains the layered node-tree model, source priorities, the generated schema, and how settings are declared in [`pkg/config/setup`](<<<SRC>>>/pkg/config/setup). [Secrets management](configuration/secrets.md) covers `ENC[...]` resolution through external backends. [Runtime settings](configuration/runtime-settings.md) covers `agent config set` and the settings API. [Remote configuration](configuration/remote-config.md) covers the Uptane/TUF-verified channel through which the Datadog backend pushes configuration to agents and tracers at runtime.

### Data pipelines

The [overview](pipelines/overview.md) maps every product pipeline end to end. Metrics are covered by [aggregation](pipelines/metrics/aggregation.md) (demultiplexer, time and check samplers, context keys) and [serialization](pipelines/metrics/serialization.md) (protobuf payloads, compression, splitting). The [logs pipeline](pipelines/logs.md) covers sources, launchers, tailers, and the processor/sender chain; the [trace pipeline](pipelines/traces.md) covers the receiver, obfuscation, sampling, and APM stats; the [process pipeline](pipelines/processes.md) covers live processes and containers, including the realtime mode driven by intake responses. [Forwarder and resilience](pipelines/forwarder.md) and the [event platform forwarder](pipelines/event-platform.md) cover the last hop, and [network device monitoring](pipelines/ndm.md) covers the SNMP, NetFlow, and traps servers embedded in the core agent. [DogStatsD internals](dogstatsd/internals.md) details the StatsD intake path.

### Checks

The check system turns integration configurations into periodically scheduled collection code. The [check collector](checks/collector.md) covers the scheduler, runner, and sender API; [Go core checks](checks/corechecks.md), [Python checks and rtloader](checks/python.md), and [JMX checks](checks/jmx.md) cover the three check runtimes; [Autodiscovery](checks/autodiscovery.md) covers how container and Kubernetes events generate check configurations dynamically.

### Containers and Kubernetes

[Workloadmeta](containers/workloadmeta.md) is the in-memory store of workload entities (containers, pods, tasks, images) fed by per-runtime collectors; the [Tagger](containers/tagger.md) derives tags from it; [origin detection](containers/origin-detection.md) attributes incoming StatsD and OTLP data to the emitting container. The [Cluster Agent](containers/cluster-agent.md) page covers the DCA's role, leader election, and its API for node agents; [cluster checks](containers/cluster-checks.md), the [admission controller](containers/admission-controller.md), [autoscaling](containers/autoscaling.md), and the [orchestrator explorer](containers/orchestrator.md) cover its major leader-only subsystems.

### eBPF and security

[system-probe](ebpf/system-probe.md) is the privileged module host with a three-way eBPF loading strategy (CO-RE, runtime compilation, prebuilt). [Network monitoring](ebpf/network-monitoring.md) covers NPM connection tracing and USM protocol inspection. [Workload Protection](ebpf/cws.md) covers the CWS event pipeline from kernel probes through the SECL rule engine to the security-agent. [Compliance and SBOM](ebpf/compliance.md) covers the CSPM benchmark engine and Trivy-based SBOM generation.

### OpenTelemetry

[OTLP ingest](otel/otlp-ingest.md) is the small hardcoded OTel Collector embedded in the core agent (ports 4317/4318). The [DDOT collector](otel/ddot.md) is the full, user-configurable Datadog distribution of the OTel Collector shipped as the `otel-agent` binary, which reuses the Agent's own metrics, logs, and trace pipelines instead of the upstream exporters.

### Deployment

[Packaging](deployment/packaging.md) covers the deb/rpm/MSI/macOS build channels (Omnibus and its Bazel successor). [Fleet automation and the installer](deployment/fleet.md) covers OCI packages, the experiment/stable symlink scheme, and remote upgrades with automatic rollback. [Container images](deployment/container-images.md) covers the Dockerfiles and s6 supervision. [Runtime environments](deployment/environments.md) covers feature detection in [`pkg/config/env`](<<<SRC>>>/pkg/config/env) and how host, Docker, Kubernetes, ECS, and Fargate deployments change process topology and behavior.

### Operations

[Status, health, and telemetry](operations/introspection.md) covers the federated status system, health probes, and the Agent's self-telemetry. [Flare](operations/flare.md) covers the scrubbed support archive and how components contribute to it. [Diagnostics and CLI tools](operations/diagnostics.md) covers the diagnose suites and the CLI subcommands, which are thin authenticated HTTP clients of the running agent.

## Scope and related documentation

These pages document architecture: process topology, control and data flow, the contracts between subsystems, and the non-obvious behaviors that matter when you change them. They do not cover environment setup, build instructions, or contribution workflow — for those, see the [developer guide](https://datadoghq.dev/datadog-agent/setup/required/) and the [guidelines](https://datadoghq.dev/datadog-agent/guidelines/contributing/) on the main developer documentation site. User-facing product documentation lives at [docs.datadoghq.com](https://docs.datadoghq.com/agent/).
