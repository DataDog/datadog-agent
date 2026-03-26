# Package List (`pkg/`)

Sub-packages that are purely internal implementation details are grouped under their parent.
Only sub-packages with a distinct, standalone purpose are called out individually.

---

## aggregator
Metrics aggregation pipeline. Receives metrics from checks and flushes them to the forwarder.
- `ckey` — Context key hashing for metric identity
- `internal` — Internal aggregation buffers
- `mocksender` — Mock sender for tests
- `sender` — Sender interface for checks to emit metrics

## api
HTTP API client utilities shared across agent binaries.
- `security` — Auth token handling
- `util` — HTTP client helpers
- `coverage` / `version` — Coverage endpoint and version info

## cli
CLI framework utilities.
- `standalone` — Standalone (one-shot) check runner CLI
- `subcommands` — Reusable subcommand definitions

## cloudfoundry
Cloud Foundry integration.
- `containertagger` — Tags containers with CF application metadata

## clusteragent
Kubernetes Cluster Agent logic (runs in the cluster-agent binary).
- `admission` — Admission controller webhooks
  - `controllers` — Kubernetes controllers: `secret` (TLS cert management), `webhook` (webhook registration)
  - `mutate` — Pod mutation webhooks:
    - `autoinstrumentation` — APM auto-instrumentation injection (adds init containers)
    - `agent_sidecar` — Datadog agent sidecar injection
    - `appsec` — AppSec injection
    - `cwsinstrumentation` — CWS (Cloud Workload Security) injection
    - `autoscaling` — Autoscaling-related mutations
    - `config` — Config injection (env vars, labels)
    - `tagsfromlabels` — Tag injection from pod labels
  - `validate` — Pod validation webhooks: `kubernetesadmissionevents`
  - `patch` — Remote patch application (Library Injection config)
  - `probe` — Admission probe/health check
  - `metrics` — Admission controller metrics
- `api` — Cluster agent HTTP API; `v1/` for v1 endpoints
- `appsec` — AppSec in cluster-agent context
  - `config` — AppSec configuration
  - `envoygateway` — Envoy Gateway integration
  - `istio` — Istio mesh integration
  - `sidecar` — AppSec sidecar management
- `autoscaling` — Autoscaling support
  - `custommetrics` — Custom metrics provider (HPA external metrics)
  - `externalmetrics` — External metrics server; `model/`
  - `cluster` — Cluster-level autoscaling; `model/`
  - `workload` — Workload autoscaling (WPA/DPCA):
    - `common` / `model` / `metrics` / `profile` — Core workload autoscaling
    - `external` — External metrics source
    - `loadstore` — Load metric store
    - `local` — Local metrics source
    - `provider` — Metrics provider interface
- `clusterchecks` — Cluster-check dispatching and load balancing; `types/`
- `evictor` — Pod evictor for rebalancing cluster checks
- `languagedetection` — Language detection server (receives data from node agents)
- `mcp` — Model Context Protocol server; `tools/` for MCP tool implementations
- `metricsstatus` — HPA metrics status provider
- `metricsstore` — In-memory store for external metrics
- `orchestrator` — Orchestrator resource collection from cluster
- `patcher` — Remote library injection patch management
- `telemetry` — Cluster agent telemetry

## collector
Check scheduling, loading, and execution engine.
- `check` — Check interface and types
- `corechecks` — Built-in Go checks, organized by domain:
  - `system` — Host system checks: `cpu` (utilization + load), `disk` (disk + io, v1 and v2), `memory`, `uptime`, `filehandles`, `battery`
  - `system` (Windows-only) — `wincrashdetect`, `windowscertificate`, `winkmem`, `winproc`
  - `net` — Network checks: `network` (v1/v2), `ntp`, `wlan`
  - `containers` — Container runtime checks: `containerd`, `cri`, `docker`, `generic` (shared), `kubelet`
  - `containerimage` — Container image metadata check
  - `containerlifecycle` — Container start/stop lifecycle events check
  - `cluster` — Kubernetes cluster-level checks: `ksm` (kube-state-metrics), `helm`, `kubernetesapiserver`, `orchestrator`
  - `orchestrator` — Orchestrator checks: `pod`, `ecs`, `kubeletconfig`
  - `ebpf` — eBPF-based checks: `oomkill`, `tcpqueuelength`, `noisyneighbor`, `ebpfcheck`; `probe/` contains the system-probe-side counterparts
  - `gpu` — GPU metrics check (NVML + eBPF); `nvidia/jetson` for Jetson boards
  - `nvidia` — NVIDIA Jetson-specific check
  - `snmp` — SNMP check with full internal pipeline (`checkconfig`, `devicecheck`, `discovery`, `fetch`, `lldp`, `metadata`, `profile`, `report`, `session`, `valuestore`)
  - `network-devices` — Network device vendor checks: `cisco-sdwan`, `versa`
  - `networkpath` — Network path check (traceroute)
  - `networkconfigmanagement` — Network config management check
  - `oracle` — Oracle DB check
  - `cloud/hostinfo` — Cloud host info check
  - `discovery` — Service discovery check
  - `sbom` — SBOM (software bill of materials) check
  - `systemd` — systemd unit check
  - `embed` — Embedded sub-process checks: `apm` (trace-agent), `process`
  - `agentprofiling` — Agent self-profiling check
  - `telemetry` — Agent telemetry check
- `loaders` — Check loaders (Go, Python, JMX, etc.)
- `python` — CPython embedding and Python check runner
- `runner` — Concurrent check runner
- `rustchecks` — Rust-based check loader
- `sharedlibrary` — Shared library (`.so`/`.dll`) check loader; `ffi/` and `sharedlibraryimpl/` are internal parts of this loader
- `scheduler` — Check scheduling (intervals, jitter)
- `worker` — Worker goroutine pool for check execution
- `aggregator` — Bridge between collector and aggregator
- `externalhost` — External host info reported by checks

## commonchecks
Shared check helpers reused by multiple built-in checks.

## compliance
Compliance monitoring (CIS benchmarks, CSPM).
- `aptconfig` — APT configuration compliance checks
- `dbconfig` — Database configuration checks
- `k8sconfig` — Kubernetes configuration checks
- `scap` — SCAP scanner integration
- `types` / `metrics` / `utils` — Shared types, metrics, and utilities

## config
Configuration management — the largest and most layered package.
- `model` — Config model interface (`ConfigReader`, `ConfigWriter`, `Config`)
- `setup` — Config key registration and defaults for all agent settings (the authoritative list of every config key); `constants/` holds shared constant values
- `nodetreemodel` — Node-tree config backend (the default in-memory implementation)
- `viperconfig` — Viper-backed config implementation (legacy)
- `teeconfig` — Tee config that broadcasts writes to multiple backends simultaneously
- `remote` — Remote Configuration-backed config overlay
  - `api` — RC gRPC API client
  - `client` — RC client (subscribes to product configs)
  - `data` — RC product/config type definitions
  - `meta` — RC TUF metadata handling
  - `rcwebsocket` — WebSocket transport for RC
  - `service` — RC service (server-side, cluster-agent)
  - `uptane` — TUF/Uptane verification logic
- `autodiscovery` — Autodiscovery config loading for checks
- `env` — Environment detection (cloud provider, container runtime, Kubernetes)
- `fetcher` — Cross-process config fetching via IPC (agent → system-probe, tracers)
  - `sysprobe` — Fetcher for system-probe config
  - `tracers` — Fetcher for tracer config
- `settings` — Runtime-settable config keys (read/write via API)
  - `http` — HTTP handlers for settings endpoint
- `legacy` — Migration helpers from old config formats (Agent 5 → 6)
- `mock` — Test mock config (`NewMock()`)
- `render_config` — Config file rendering (template → yaml)
- `basic` / `create` / `helper` / `structure` / `utils` — Config construction and introspection helpers

## containerlifecycle
Container lifecycle event collection and forwarding (start/stop/OOM events).

## databasemonitoring
Database monitoring agent-side logic.
- `aws` — AWS RDS/Aurora instance discovery

## diagnose
Agent self-diagnostics framework.
- `connectivity` — Network connectivity checks to Datadog endpoints
- `firewallscanner` — Firewall rule detection
- `ports` — Local port availability checks

## discovery
Service and application discovery (used by USM and APM auto-instrumentation).
- `apm` — APM service discovery (detects APM-instrumented services)
- `core` — Core discovery engine (orchestrates all discovery sources)
- `envs` — Environment variable inspection for discovery
- `language` — Language detection heuristics (inspects ELF, symbols, etc.)
- `model` — Shared discovery data model (service, language, endpoint)
- `module` — System-probe module for discovery
  - `rust/` — Rust component for low-level process inspection (with `include/`, `src/`, tests)
  - `splite` — Split/lite mode for the discovery module
- `tracermetadata` — Tracer metadata extraction from running processes
  - `language/` — Language-specific tracer metadata
- `usm` — Universal Service Monitoring service discovery

## dyninst
Dynamic instrumentation (DI / Live Debugger) — attaches eBPF uprobes to running Go processes to capture variable values at probe points without restarting.

### Configuration & Remote Config
- `rcjson` — Data structures for Remote Configuration DI service payloads (probe definitions received from the backend)
- `exprlang` — Expression language DSL — parses and evaluates probe capture expressions (e.g. `arg0`, `this.field`)
- `process` — Instrumentation configuration for a process (maps RC config to per-process probe state)
- `procsubscribe` — Subscribes to process start/stop events and RC updates to trigger instrumentation changes

### IR (Intermediate Representation)
- `ir` — Core IR: `ir.Program` represents all probes applied to a single binary — the central data structure that flows through the pipeline
- `irgen` — Generates an `ir.Program` from DWARF debug info + probe config; uses Go ABI knowledge to locate function arguments
- `irprinter` — Serializes IR to JSON for debugging and testing (not stable API)
- `compiler` — Compiles `ir.Program` into a stack-machine bytecode program loaded into eBPF maps

### Binary & Symbol Analysis
- `object` — Parses ELF object files; extracts DWARF debug sections with decompression and disk caching
- `dwarf` — DWARF format constants and low-level parsing helpers
- `gotype` — Reverse-engineers Go type information from ELF binaries (relies on Go compiler internals)
- `gosym` — Go symbol table (`symtab` + `pcln` table) parsing — higher-performance alternative to `debug/gosym`
- `symbol` — Symbolicator: resolves program counter addresses to function names and source locations
- `symdb` — Processes DWARF debug info to extract Go symbol information for upload to SymDB

### eBPF Program Lifecycle
- `ebpf` — eBPF C programs and BPF maps for dyninst (the kernel-side stack machine)
- `loader` — Loads the eBPF program, applies relocations, and prepares it for attachment
- `uprobe` — Attaches and detaches uprobes to specific function entry points in target processes

### Runtime Orchestration
- `actuator` — Top-level orchestrator: coordinates IR compilation, eBPF loading, and uprobe attachment for each instrumented process; implements circuit-breaker to enforce CPU limits
- `dispatcher` — Forwards raw eBPF ring-buffer events from the kernel to the appropriate consumer (sink)
- `module` — System-probe module entry point; wires actuator, dispatcher, and RC subscription together

### Data Extraction & Output
- `decode` — Decodes raw eBPF output events (stack-machine results) into structured Go values using the IR
- `output` — Interprets decoded values and produces final probe output events (JSON snapshots of captured variables)
- `uploader` — Batches and uploads probe output events to the Datadog backend

### Utilities
- `htlhash` — Computes the HTL (head-tail-length) hash of an executable file — used as a build ID per the OTel profiles spec
- `dyninsttest` — Shared test helpers for dyninst integration tests
- `testprogs` / `testdata` — Test target programs and fixtures
- `ebpfbench` / `trietest` — Benchmarks and trie-specific tests

## ebpf
eBPF infrastructure shared by all eBPF-based features.
- `bytecode` — Compiled eBPF object management (prebuilt + CO-RE)
- `c` — Shared eBPF C headers
- `compiler` — Runtime eBPF compilation
- `features` — Kernel feature detection
- `kernelbugs` — Known kernel bug workarounds
- `maps` — eBPF map wrappers
- `perf` — Perf event ring-buffer reader
- `telemetry` — eBPF map/program telemetry
- `uprobes` — Uprobe attachment helpers
- `verifier` — eBPF verifier complexity analysis

## errors
Common error types and helpers used across the agent.

## eventmonitor
Kernel event monitoring framework (wraps security probe for general use).
- `config` — Event monitor configuration
- `consumers` — Event consumer interface and built-in consumers

## fips
FIPS 140-2 compliance mode detection and enforcement.

## flare
Diagnostic flare (support bundle) generation.
- `common` — Shared flare collection logic
- `clusteragent` — Cluster-agent-specific flare additions
- `securityagent` — Security-agent-specific flare additions
- `priviledged` — Files requiring elevated permissions

## fleet
Fleet Automation (remote agent management and software installation).
- `daemon` — Fleet daemon that executes remote tasks from Datadog
- `installer` — Package installer and lifecycle management
  - `bootstrap` — First-time installer bootstrap (before the daemon is running)
  - `commands` — CLI commands for the installer
  - `config` — Installer configuration
  - `db` — Local SQLite state database (installed packages, versions)
  - `env` — Environment variables and runtime context
  - `exec` — Sub-process execution helpers
  - `installinfo` — Recorded installation method/source
  - `msi` — Windows MSI manipulation
  - `oci` — OCI image pull and layer extraction (packages distributed as OCI images)
  - `packages` — Per-package install/upgrade/uninstall logic:
    - `apminject` — APM auto-injection package
    - `ssi` — Single-Step Instrumentation package
    - `integrations` — Agent integration packages
    - `extensions` — Package extensions
    - `fapolicyd` — fapolicyd policy management
    - `selinux` — SELinux policy management
    - `file` / `exec` / `user` / `embedded` — Package helpers (file ops, subprocess exec, user management, embedded scripts/templates)
    - `service` — Service lifecycle management (systemd / sysvinit / upstart / windows)
    - `packagemanager` — OS package manager abstraction (apt, yum, etc.)
  - `paths` — Installer filesystem path constants
  - `repository` — Local package repository (manages on-disk versions and symlinks)
  - `setup` — Fleet setup scripts for specific environments (`djm`, `defaultscript`)
  - `symlink` / `tar` — Filesystem and archive helpers
  - `telemetry` — Installer telemetry

## gohai
System information collection (sent in host metadata).
- `cpu` / `filesystem` / `memory` / `network` / `platform` / `processes` — Per-subsystem collectors

## gpu
GPU monitoring via NVML and eBPF (CUDA kernel tracing).
- `config` — GPU check configuration
- `containers` — GPU-to-container attribution
- `cuda` — CUDA event parsing
- `ebpf` — eBPF probes for GPU kernel activity
- `safenvml` — Safe NVML wrapper (handles missing library gracefully)
- `tags` — GPU tag generation

## hosttags
Host tag collection from config, EC2, GCE, Azure, etc.

## inventory
Host software and system inventory.
- `software` — Installed software enumeration
- `systeminfo` — OS and hardware system information

## jmxfetch
JMX metric collection — manages the JMXFetch subprocess and its configuration.

## kubestatemetrics
Kubernetes state metrics collection (kube-state-metrics embedded in the agent).
- `builder` — KSM store builder
- `store` — In-memory Kubernetes object store

## languagedetection
Process language detection (Go, Python, Java, Ruby, etc.).
- `languagemodels` — Language model definitions
- `privileged` — Privileged (root) detection methods
- `util` — Shared detection utilities

## logonduration
Windows user logon duration measurement.

## logs
Log collection and processing pipeline.
- `client` — Log transport clients
  - `http` — HTTP batching client
  - `tcp` — TCP persistent connection client
- `diagnostic` — Runtime log pipeline diagnostics (stream logs to CLI)
- `launchers` — Log source launchers (one per source type):
  - `file` — File log launcher; `provider/` handles file rotation and discovery
  - `container` — Container log launcher; `tailerfactory/` selects the right tailer per runtime
  - `journald` — systemd journald launcher
  - `listener` — TCP/UDP socket listener launcher
  - `windowsevent` — Windows Event Log launcher
  - `channel` — In-process channel-based launcher (for agent-internal logs)
  - `integration` — Integration log source launcher
- `message` — Log message struct and encoding (origin, content, tags)
- `pipeline` — Processing pipeline orchestration (decode → process → send)
- `processor` — Log processors: multiline aggregation, remapping, redaction, enrichment
- `schedulers` — Autodiscovery-driven log scheduling
  - `ad` — Autodiscovery scheduler
  - `channel` — Channel-based scheduler
- `sender` — Batching and retry sender
  - `http` / `tcp` — Protocol-specific sender backends
- `sources` — Log source registry (tracks active and inactive sources)
- `tailers` — Tailer implementations (one per source type):
  - `file` — File tailer (byte-offset tracking, rotation handling)
  - `container` — Container stdout/stderr tailer
  - `journald` — journald tailer
  - `socket` — TCP/UDP socket tailer
  - `windowsevent` — Windows Event Log tailer
  - `channel` — In-process channel tailer
- `internal` — Internal implementation details:
  - `decoder` — Log line decoder (framing, multiline, preprocessing)
  - `framer` — Line framing strategies (newline, length-prefix, etc.)
  - `parsers` — Log format parsers: `dockerfile`, `dockerstream`, `encodedtext`, `integrations`, `kubernetes`, `noop`
  - `tag` — Tag provider for log messages
  - `util` — Internal utilities: `adlistener`, `containersorpods`, `opener`
- `metrics` — Internal log pipeline metrics
- `service` — Log service lifecycle
- `status` — Log agent status (active sources, errors)
- `types` — Shared log types (status codes, etc.)
- `util` — Utilities: `opener` (file handle management), `windowsevent`, `testutils`

## metrics
Core metric type definitions (gauges, counts, histograms, etc.).
- `event` — Datadog event type
- `servicecheck` — Service check type

## network
Network performance monitoring (NPM) — TCP/UDP connection tracking via eBPF.
- `config` — NPM configuration; `sysctl/` for kernel parameter tuning
- `dns` — DNS query monitoring (intercepts DNS responses via eBPF)
- `driver` — Windows network driver interface (WFP-based)
- `ebpf` — eBPF programs and maps for connection tracking
  - `c/` — C source: `tracer/`, `conntrack/`, `protocols/`, `shared-libraries/`, CO-RE variants
  - `probes/` — eBPF probe definitions
- `encoding` — Protobuf encoding/decoding for the connections payload
  - `marshal` / `unmarshal` — Encode and decode connection stats
- `filter` — Connection filtering (allowed/blocked networks, local CIDRs)
- `go` — Go binary inspection for TLS tracing (reads DWARF/symbols at runtime)
  - `asmscan` — Assembly scanner
  - `bininspect` — Binary inspector (struct offsets, versions)
  - `binversion` — Go binary version extraction
  - `dwarfutils` — DWARF debug info utilities
  - `goid` — Goroutine ID extraction
  - `goversion` — Go version detection
  - `lutgen` / `rungo` — Lookup table generator and Go runtime helpers
- `netlink` — Netlink-based conntrack for NAT translation
- `protocols` — Application-layer protocol classification (USM protocols)
  - `http` — HTTP/1.x tracing (including `gotls/` for Go TLS)
  - `http2` — HTTP/2 tracing
  - `kafka` — Kafka protocol tracing
  - `postgres` — PostgreSQL protocol tracing
  - `mysql` — MySQL protocol tracing
  - `redis` — Redis protocol tracing
  - `mongo` — MongoDB protocol tracing
  - `amqp` — AMQP protocol tracing
  - `tls` — TLS classification; `gotls/` and `nodejs/` for language-specific TLS
  - `events` — eBPF protocol event ring-buffer consumer
  - `telemetry` — Per-protocol telemetry metrics
- `tracer` — Core connection tracer (entry point for NPM)
  - `connection` — Connection tracking backends: `kprobe`, `fentry`, `ebpfless`
  - `offsetguess` — Kernel struct offset detection for eBPF
  - `networkfilter` — Network-level connection filter
- `usm` — Universal Service Monitoring (protocol-level traffic stats)
  - `config` / `consts` / `maps` / `state` — USM internals
  - `sharedlibraries` — Shared library (OpenSSL, etc.) tracking for uprobes
  - `procnet` — `/proc/net` socket tracking
  - `debugger` — USM debug tool
  - `buildmode` — Build mode selection (CO-RE vs prebuilt)
  - `utils` — Shared USM utilities
- `containers` / `events` / `indexedset` / `payload` / `sender` / `slice` / `types` — Supporting sub-packages

## networkconfigmanagement
Network device configuration management (push configs to devices).
- `config` / `profile` / `remote` / `report` / `sender` — Pipeline stages

## networkdevice
Shared types and utilities for NDM (Network Device Monitoring).
- `diagnoses` — Device diagnostic results
- `integrations` — Integration metadata
- `metadata` — Device metadata payload
- `pinger` — ICMP pinger for device reachability
- `profile` — SNMP device profile definitions
- `sender` — Metric sender helpers

## networkpath
Network path tracing (traceroute-based hop analysis).
- `traceroute` — Platform-specific traceroute implementations
- `metricsender` / `payload` / `telemetry` — Supporting sub-packages

## obfuscate
Sensitive data obfuscation (SQL, Redis, MongoDB queries; stack traces).

## opentelemetry-mapping-go
OpenTelemetry ↔ Datadog data model mapping.
- `inframetadata` — OTel resource attributes → Datadog host metadata
  - `gohai` — gohai-format payload builder from OTel resources
  - `payload` — Infra metadata payload types
- `otlp` — OTLP data → Datadog format conversion
  - `attributes` — OTel resource/span attribute mapping
    - `azure` / `ec2` / `gcp` — Cloud provider attribute normalization
    - `source` — Source (host) attribute extraction
  - `metrics` — OTLP metrics → Datadog metrics conversion
  - `logs` — OTLP logs → Datadog logs conversion
  - `rum` — OTLP → Datadog RUM conversion

## orchestrator
Orchestrator Explorer — collects and streams Kubernetes/ECS resource manifests.
- `config` — Orchestrator configuration
- `model` — Payload models
- `util` — Shared utilities

## persistentcache
Simple on-disk key-value cache used by checks to persist state across restarts.

## pidfile
PID file creation and cleanup.

## privateactionrunner
Private Action Runner — executes Datadog workflow actions inside private networks.
- `adapters` — Adapters that bridge the runner core to external systems:
  - `actions` / `config` / `constants` / `httpclient` / `logging` / `modes` / `parversion` / `rcclient` / `regions` / `tmpl` / `workflowjsonschema`
- `autoconnections` — Automatic connection management; `conf/` for connection config
- `bundle-support` — Shared support libraries for bundles:
  - `gitlab` — GitLab API client shared by bundles
  - `httpclient` — HTTP client shared by bundles
  - `kubernetes` — Kubernetes client shared by bundles
- `bundles` — Action bundle implementations (one bundle = one integration):
  - `ddagent` — Datadog agent actions (e.g. `networkpath`)
  - `gitlab` — GitLab actions: `branches`, `commits`, `deployments`, `environments`, `issues`, `jobs`, `labels`, `members`, `mergerequests`, `notes`, `pipelines`, `projects`, `protectedbranches`, `repositories`, `repositoryfiles`, `tags`, `users`, `customattributes`, `graphql`
  - `kubernetes` — Kubernetes actions: `core`, `apps`, `batch`, `apiextensions`, `discovery`, `customresources`
  - `http` — Generic HTTP action
  - `jenkins` — Jenkins actions
  - `mongodb` — MongoDB actions
  - `remoteaction` — Remote shell action (`rshell`)
  - `script` — Script execution action
  - `temporal` — Temporal workflow action
- `credentials` — Credential resolution; `resolver/` for credential backends
- `enrollment` — Runner enrollment and token management
- `libs` — Shared libraries:
  - `connection` — Connection management
  - `par` — PAR (Private Action Runner) protocol
  - `privateconnection` — Private network connection handling
  - `tempfile` — Temp file management
- `observability` — Metrics, tracing, and logging for the runner
- `opms` — OPMS (Operations Management) integration
- `runners` — Core action runner execution engine
- `task-verifier` — Task signature/integrity verification
- `types` / `util` — Shared types and utilities

## privileged-logs
Privileged log collection (reads kernel/system logs requiring root).
- `client` — Client for the privileged log socket
- `common` — Shared types
- `module` — System-probe module exposing privileged log stream

## process
Process monitoring (process list, connections, containers).
- `checks` — Process, container, and network connection checks (process, container, rt-container, connections, process-discovery)
- `encoding` — Payload protobuf encoding; `request/` for check request encoding
- `metadata` — Process metadata enrichment
  - `parser` — Command-line parsers for language detection: `java/`, `nodejs/`
  - `workloadmeta` — Workload metadata collector; `collector/`
- `monitor` — Process lifecycle monitor (watches for process start/stop events)
- `net` — IPC client for communicating with system-probe; `resolver/` for connection resolution
- `procutil` — Cross-platform process information utilities (PID, cmdline, stats, I/O)
- `runner` — Check runner for process agent; `endpoint/` for runner API endpoint
- `status` — Process agent status page data
- `subscribers` — Event subscribers for process lifecycle events
- `util` — Shared utilities
  - `api` — Process agent API helpers; `config/` and `headers/`
  - `containers` — Container utilities for process agent
  - `coreagent` — Core agent integration utilities
  - `status` — Status utilities

## procmgr
Process manager — tracks spawned subprocesses.
- `rust` — Rust component integration

## proto
Protobuf and MessagePack generated code.
- `pbgo` — Generated Go protobuf types
- `msgpgo` — MessagePack-encoded Go types
- `datadog` — Datadog-specific proto definitions
- `utils` — Proto helper utilities

## redact
Sensitive data redaction for logs, traces, and config values.

## remoteconfig
Remote Configuration client-side state machine.
- `state` — RC state management (targets, client cache, repository)

## runtime
Runtime utilities (goroutine introspection, memory stats).

## sbom
Software Bill of Materials scanning and reporting.
- `collectors` — SBOM collectors (container images, host packages)
- `scanner` — SBOM scan orchestration
- `bomconvert` — BOM format conversion
- `telemetry` / `types` — Supporting sub-packages

## security
Cloud Workload Security (CWS) / CSPM agent — the largest package.
- `probe` — Kernel-space event probe (eBPF programs for syscall monitoring)
  - `constantfetch` — Kernel struct constant fetching (BTF, BTFHub, fallback); `btfhub/` for BTFHub integration
  - `erpc` — eBPF ring-buffer to userspace RPC communication
  - `eventstream` — Kernel event stream consumer; `reorderer/` and `ringbuffer/` backends
  - `kfilters` — Kernel-side event pre-filtering (approvers/discarders)
  - `managerhelper` — eBPF manager helpers
  - `monitors` — Probe-internal monitors: `approver`, `cgroups`, `discarder`, `dns`, `eventsample`, `syscalls`
  - `procfs` — `/proc` filesystem reader for process info
  - `selftests` — Probe self-test suite (verifies eBPF is working at startup)
  - `sysctl` — Kernel sysctl tuning
  - `config/` — Probe-specific configuration
- `module` — System-probe module that wraps the probe and exposes gRPC API
- `agent` — Security agent (user-space; consumes events and applies rules)
- `secl` — Security Evaluation and Control Language — the rule DSL
  - `compiler` — SECL compiler: `ast/` (parser) and `eval/` (expression evaluator)
  - `model` — Event data model (all field types, generated accessors); `bpf_maps_generator/`, `sharedconsts/`, `usersession/`, `utils/`
  - `rules` — Rule and policy model; `filter/` for rule filtering
  - `schemas` — JSON schemas for policy files
  - `args` / `containerutils` / `log` / `utils` / `validators` — Utilities
- `seclwin` — Windows-specific SECL model; `model/` for Windows event types
- `rules` — Rule loading, evaluation, and policy management
  - `bundled` — Bundled (built-in) rules
  - `filtermodel` — Filter model for rule matching
  - `monitor` — Rule evaluation monitor
- `resolvers` — Kernel object resolvers (translate kernel IDs to rich metadata):
  - `process` — Process resolver (PID → process tree)
  - `dentry` — Dentry (file path) resolver
  - `file` — File metadata resolver
  - `mount` — Mount point resolver
  - `cgroup` — cgroup resolver; `model/` for cgroup types
  - `dns` — DNS resolver
  - `envvars` — Process environment variable resolver
  - `hash` — File hash resolver
  - `netns` — Network namespace resolver
  - `path` — Path resolver (combines dentry + mount)
  - `sbom` — SBOM resolver; `collectorv2/` and `types/`
  - `selinux` — SELinux label resolver
  - `sign` — Binary signature resolver
  - `syscallctx` — Syscall context resolver
  - `tags` / `tc` / `usergroup` / `usersessions` / `securitydescriptors` — Additional resolvers
- `security_profile` — Anomaly detection and behavioral security profiles
  - `activity_tree` — Activity tree (process/file/network activity graph); `metadata/`
  - `dump` — Profile dump (serialization/deserialization)
  - `profile` — Profile management and anomaly scoring
  - `storage` — Profile storage backends; `backend/`
- `process_list` — Shared process list and activity tree
  - `activity_tree` — Activity tree node types
  - `process_resolver` — Process resolver used by both probe and profiles
- `ebpf` — eBPF C programs and BPF maps for CWS
  - `c/` — C source: `include/`, `prebuilt/`, `runtime/`
  - `kernel/` — Kernel feature detection for CWS
  - `probes/` — eBPF probe definitions; `rawpacket/` for raw packet capture
- `ptracer` — ptrace-based tracer (fallback for environments without eBPF)
- `proto` — CWS-specific protobuf definitions
  - `api` — gRPC API; `mocks/` and `transform/`
  - `ebpfless` — eBPF-less tracer proto types
- `serializers` — Event payload serialization (JSON for SIEM/backend)
- `generators` — Code generators:
  - `accessors` — Generate SECL field accessor code
  - `backend_doc` — Generate backend documentation
  - `event_copy` / `event_deep_copy` / `operators` / `schemas` / `syscall_table_generator`
- `events` — Event type definitions and constants
- `config` — CWS configuration
- `common` — Shared types; `usergrouputils/`
- `metrics` — CWS-specific metric names
- `rconfig` — Remote Configuration integration for CWS rules
- `reporter` — Event reporter (sends to backend)
- `seclog` — Security-specific structured logger
- `telemetry` — CWS telemetry metrics
- `utils` — Shared CWS utilities: `cache/`, `grpc/`, `k8sutils/`, `lru/`, `pathutils/`
- `clihelpers` — CLI command helpers for security-agent

## serializer
Agent payload serialization (JSON, MessagePack, Protocol Buffers).
- `marshaler` — Marshaler interface
- `split` — Payload splitting for size limits
- `internal` / `mocks` / `types` — Supporting internals

## serverless
Serverless (AWS Lambda) agent.
- `logs` — Lambda log collection via extension API
- `metrics` — Enhanced Lambda metrics
- `trace` — Lambda trace collection
- `otlp` — OTLP ingestion in serverless context
- `streamlogs` — Log streaming
- `env` / `tags` — Environment and tag utilities

## snmp
SNMP shared utilities.
- `gosnmplib` — goSNMP library wrappers
- `snmpintegration` — Integration config types
- `snmpparse` — SNMP config parsing
- `devicededuper` — Device deduplication
- `utils` — Shared SNMP utilities

## ssi
Single Step Instrumentation (auto-inject APM libraries).

## status
Agent status page rendering.
- `health` — Component health registry
- `render` — Status HTML/JSON rendering
- `clusteragent` / `collector` / `endpoints` / `httpproxy` / `jmx` / `systemprobe` — Per-subsystem status providers

## system-probe
System-probe daemon interface (runs as a separate privileged process).
- `api` — IPC API between agent and system-probe
- `config` — System-probe configuration
- `utils` — Shared utilities

## tagger
Entity tagging — enriches metrics/logs/traces with container and infrastructure tags.
- `types` — Tag cardinality and entity ID types

## tagset
Efficient, immutable tag set data structures with hash-based deduplication.

## telemetry
Prometheus/OpenMetrics telemetry for internal agent metrics (exposed on `/telemetry`).

## template
Template rendering utilities.
- `html` / `text` — HTML and text template wrappers

## trace
APM trace-agent core — receives, processes, samples, and forwards traces.
- `agent` — Top-level trace agent orchestration (start/stop, wires all components)
- `api` — Trace intake HTTP/gRPC API (receives spans from tracers)
  - `apiutil` — API utility helpers
  - `loader` — Dynamic API loader
  - `internal/header` — HTTP header parsing
- `config` — Trace agent configuration (inherits from `pkg/config`)
- `sampler` — Priority sampling, error sampling, and rate-limiting
- `stats` — APM stats computation (Concentrator — computes p50/p75/p99 from spans)
- `filters` — Trace filtering rules (block/allow by resource, service, etc.)
- `payload` — Trace payload encoding and transport types
- `pb` — Generated protobuf types for spans and traces
- `writer` — Trace and stats writers (batching, retry, compression, send to backend)
- `event` — APM event (error/rare trace) extraction logic
- `otel` — OpenTelemetry trace ingestion
  - `integration` — OTel receiver integration
  - `stats` — OTel → APM stats conversion
  - `traceutil` — OTel trace utilities
- `remoteconfighandler` — Remote Configuration integration (dynamic sampling rates, block lists)
- `traceutil` — Trace utility functions (normalization, truncation)
  - `normalize` — Span field normalization
- `transform` — Span and trace transformation (OTel → DD, enrichment)
- `semantics` — Semantic convention helpers (span kind, peer tags)
- `containertags` — Container tag resolution for spans
- `info` — Runtime info and stats (expvar, status page data)
- `log` — Trace-agent-specific logger adapter
- `telemetry` — Internal telemetry metrics
- `timing` — Latency timing utilities
- `version` — Trace-agent version info
- `watchdog` — Resource watchdog (CPU/memory limits, auto-restart)
- `teststatsd` / `testutil` — Test helpers

## util
Large collection of generic utilities. Grouped by concern below.

### Cloud providers
- `aws` — AWS SDK helpers and credential utilities
  - `creds` — AWS credential providers (with `tags` for tag-based credential selection)
- `ec2` — EC2 instance metadata (IMDSv1/v2), tags, and payloads
- `ecs` — ECS task metadata client (v1/v2/v3/v4 APIs), common helpers, telemetry
- `fargate` — AWS Fargate detection and metadata
- `cloudproviders` — Multi-cloud provider detection and tag collection
  - `alibaba` / `azure` / `gce` / `ibm` / `oracle` / `tencent` / `cloudfoundry` — Per-provider implementations
  - `network` / `kubernetes` — Cloud-provider network and Kubernetes detection helpers

### Kubernetes
- `kubernetes` — Kubernetes client utilities
  - `apiserver` — Kubernetes API server client, controllers, and leader election
  - `autoscalers` — HPA/custom-metrics autoscaler helpers
  - `certificate` — TLS certificate management for webhooks
  - `cloudprovider` — Cloud-provider-specific Kubernetes helpers
  - `clusterinfo` / `clustername` / `hostinfo` — Cluster metadata resolution
  - `kubelet` — Kubelet API client (with mock)
- `kubelet` — Standalone kubelet client (simpler interface used outside cluster-agent)

### Container runtimes
- `containerd` — containerd gRPC client (with `fake` for tests)
- `docker` — Docker daemon client (with `fake` for tests)
- `crio` — CRI-O runtime client
- `podman` — Podman socket client
- `containers` — Shared container metadata types and utilities
  - `cri` — CRI (Container Runtime Interface) client and mock
  - `image` — Container image name parsing
  - `metadata` — Container metadata model
  - `metrics` — Container metrics collection, with per-runtime providers:
    - `containerd` / `cri` / `docker` / `ecsfargate` / `ecsmanagedinstances` / `kubelet` / `system`
    - `provider` — Multi-backend metrics provider abstraction
- `cgroups` — cgroup v1/v2 parsing (CPU, memory, blkio)
  - `memorymonitor` — OOM and memory pressure monitor

### Networking & HTTP
- `http` — HTTP client with retries, proxy support, and TLS configuration
- `grpc` — gRPC client helpers
  - `context` — gRPC context utilities
- `net` — Low-level network utilities (interfaces, IPs, sockets)
- `port` — Port availability checks
  - `portlist` — System port list enumeration

### Logging
- `log` — Structured logger (wraps seelog); used everywhere in the agent
  - `setup` — Logger initialization from config
  - `slog` — `log/slog`-compatible adapter
    - `filewriter` / `formatters` / `handlers` — slog backend implementations
  - `syslog` — Syslog output backend
  - `types` — Log level types
  - `zap` — Zap logger adapter

### System & OS
- `kernel` — Kernel version detection and parsing
  - `headers` — Kernel header fetching for eBPF compilation
  - `netns` — Network namespace enumeration
- `filesystem` — File system utilities (permissions, atomic writes)
- `executable` — Current executable path detection
- `os` — OS abstraction utilities
- `system` — System-level helpers (CPU count, memory info)
  - `socket` — Unix domain socket helpers
- `cgroups` — (see Container runtimes above)
- `dmi` — DMI/SMBIOS hardware info (UUID, vendor, serial)
- `lsof` — Open file descriptor listing
- `procfilestats` — `/proc` file statistics reader
- `ktime` — Kernel monotonic → wall-clock time conversion
- `coredump` — Core dump configuration
- `crashreport` — Agent crash report collection

### Windows-specific
- `pdhutil` — Windows Performance Data Helper (PDH) API wrapper
- `winutil` — General Windows utilities
  - `etw` — ETW (Event Tracing for Windows) consumer
  - `eventlog` — Windows Event Log reader
    - `api` / `bookmark` / `session` / `subscription` / `reporter` / `publishermetadatacache` — Event Log sub-components
  - `iisconfig` — IIS configuration reader
  - `iphelper` — Windows IP Helper API (ARP, routing tables)
  - `messagestrings` — Windows message string resources
  - `servicemain` — Windows service lifecycle helpers
  - `winmem` — Windows memory utilities
  - `datadoginterop` — Datadog ↔ Windows interop helpers

### Data structures & algorithms
- `cache` — TTL in-memory cache
- `cachedfetch` — Cached value fetcher with TTL and refresh
- `quantile` — DDSketch quantile estimation
  - `summary` — Summary statistics on top of DDSketch
- `trie` — Trie data structure
- `maps` — Generic map utilities
- `slices` — Generic slice utilities
- `sort` — Sort helpers
- `strings` — String manipulation helpers
- `pointer` — Pointer conversion helpers
- `option` — Optional value type
- `intern` — String interning pool
- `buf` — Byte buffer pool
- `aggregatingqueue` — Aggregating queue for batching items before flush
- `size` — Human-readable size formatting
- `stat` — Stat helpers

### Concurrency & lifecycle
- `startstop` — Component `Start()`/`Stop()` lifecycle helpers
- `retry` — Exponential backoff retry with jitter
- `backoff` — Backoff policy primitives
- `subscriptions` — Type-safe pub/sub event bus
- `sync` — Synchronization primitives (once, mutex wrappers)
- `workqueue` — Work queue with telemetry
  - `telemetry` — Work queue telemetry metrics
- `funcs` — Function memoization and lazy initialization helpers
- `statstracker` — Rolling window stats tracker
- `utilizationtracker` — Per-component CPU utilization tracking
- `atomicstats` — Atomic counter aggregation

### Observability
- `scrubber` — Sensitive credential scrubber (used for logs and flares)
- `profiling` — Continuous profiling client helpers
- `goroutinesdump` — Goroutine stack dump on signal
- `prometheus` — Prometheus metrics helpers
- `otel` — OpenTelemetry SDK helpers

### Agent-specific utilities
- `clusteragent` — Cluster agent IPC client (used by node agents)
- `hostname` — Hostname resolution with caching and providers
  - `validate` — Hostname validation rules
- `hostinfo` — Aggregated host information
- `tags` — Tag formatting, merging, and normalization
- `tmplvar` — Template variable (`%%tag%%`) substitution
- `flavor` — Agent flavor detection (`agent`, `cluster-agent`, `iot-agent`, etc.)
- `installinfo` — Installation method metadata (package, Docker, Helm, etc.)
- `defaultpaths` — Platform-specific default file and directory paths
- `fxutil` — fx dependency-injection test helpers
  - `logging` — fx startup logging
- `common` — Miscellaneous shared helpers that don't fit elsewhere

### Serialization & encoding
- `compression` — Payload compression abstraction
  - `impl-gzip` / `impl-zlib` / `impl-zstd` / `impl-zstd-nocgo` / `impl-noop` — Backend implementations
  - `selector` — Compression algorithm selector
- `archive` — Archive (zip/tar) creation and extraction
- `json` — JSON encoding helpers
- `jsonquery` — JMESPath-style JSON querying

### Miscellaneous
- `cli` — CLI formatting helpers (tables, prompts)
- `input` — Interactive user input prompts
- `safeelf` — Safe ELF binary parser (handles malformed binaries)
- `trivy` — Trivy vulnerability scanner integration
  - `walker` — File system walker for Trivy
- `gpu` — GPU utility helpers (NVML wrappers)
- `uuid` — UUID generation
- `xc` — Cross-component communication utilities
- `testutil` — Test utilities (retry helpers, fixture loading)
  - `docker` — Docker test helpers
  - `flake` — Flaky test detection helpers

## version
Agent version information (`pkg/version.AgentVersion`, build metadata).

## windowsdriver
Windows kernel driver integration.
- `driver` — Driver loading and communication
- `ddinjector` — DLL injection for APM
- `olreader` — Object list reader
- `procmon` — Process monitor driver interface
- `include` — C header files for the driver
