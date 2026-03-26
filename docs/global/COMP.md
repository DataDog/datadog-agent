# Component List (`comp/`)

All top-level component groups and their sub-components.

## agent
- `autoexit` — Handles automatic agent exit conditions
- `cloudfoundrycontainer` — Cloud Foundry container metadata
- `expvarserver` — Expvar HTTP debug endpoint
- `jmxlogger` — JMX log forwarding

## aggregator
- `demultiplexer` — Metric demultiplexer (fan-out to multiple senders)
- `demultiplexerendpoint` — HTTP endpoint for the demultiplexer

## api
- `api` — Main agent HTTP API server
- `commonendpoints` — Shared endpoint handlers
- `grpcserver` — gRPC server

## autoscaling
- `datadogclient` — Datadog API client for autoscaling

## checks
- `agentcrashdetect` — Windows agent crash detection
- `windowseventlog` — Windows Event Log check
- `winregistry` — Windows registry check

## collector
- `collector` — Check scheduling and execution engine

## connectivitychecker
- `checker` / `def` / `fx` / `impl` — Connectivity diagnostics to Datadog endpoints

## core
- `agenttelemetry` — Agent self-telemetry
- `autodiscovery` — Service/check autodiscovery
- `config` — Configuration loading and access
- `configstream` — Streaming config updates
- `configsync` — Config synchronization across processes
- `delegatedauth` — Delegated authentication
- `diagnose` — Agent diagnostics framework
- `flare` — Flare (support bundle) generation
- `fxinstrumentation` — fx dependency-injection instrumentation
- `gui` — Agent GUI server
- `healthprobe` — Health check HTTP probe
- `hostname` — Hostname resolution
- `ipc` — Inter-process communication
- `log` — Structured logging
- `lsof` — Open file descriptors listing
- `pid` — PID file management
- `profiler` — Runtime profiler
- `remoteagent` — Remote agent communication
- `remoteagentregistry` — Registry of remote agents
- `secrets` — Secret backend integration
- `settings` — Runtime settings (read/write via API)
- `status` — Agent status page
- `sysprobeconfig` — System-probe configuration
- `tagger` — Entity tagging
- `telemetry` — Prometheus/OpenMetrics telemetry
- `workloadfilter` — Workload filtering rules
- `workloadmeta` — Workload metadata store

## def
- Shared component interface definitions

## dogstatsd
- `config` — DogStatsD configuration
- `constants` — Shared constants
- `http` — HTTP API for DogStatsD
- `listeners` — UDP/UDS/named-pipe packet listeners
- `mapper` — Metric name mapping rules
- `packets` — Packet buffer pools
- `pidmap` — PID-to-container mapping
- `replay` — Traffic capture and replay
- `server` — Core DogStatsD server
- `serverDebug` — Debug endpoint for the server
- `statsd` — StatsD client used internally
- `status` — DogStatsD status page

## etw
- `impl` — Windows ETW (Event Tracing for Windows) integration

## filterlist
- `def` / `fx` / `fx-mock` / `impl` — Allow/deny filter list for workloads

## fleetstatus
- `def` / `fx` / `impl` — Fleet automation status reporting

## forwarder
- `connectionsforwarder` — Network connections forwarder
- `defaultforwarder` — Default metric/event forwarder
- `eventplatform` — Event Platform forwarder
- `eventplatformreceiver` — Event Platform receiver
- `orchestrator` — Orchestrator payload forwarder

## haagent
- `def` / `fx` / `helpers` / `impl` / `mock` — High-availability agent coordination

## healthplatform
- `def` / `fx` / `impl` / `mock` — Health platform integration

## host-profiler
- `collector` — Continuous profiler data collection
- `flare` — Profiler flare support
- `oom` — OOM event handling
- `symboluploader` — Symbol upload for profiles
- `version` — Version info for the profiler

## languagedetection
- `client` — Language detection client (sends to cluster-agent)

## logonduration
- `def` / `fx` / `impl` — Windows logon duration measurement

## logs
- `adscheduler` — Autodiscovery-based log schedule
- `agent` — Core logs agent
- `auditor` — Log auditor (tracks offsets)
- `integrations` — Integration log sources
- `streamlogs` — Log streaming

## logs-library
- `kubehealth` — Kubernetes health log source

## metadata
- `clusteragent` — Cluster agent metadata payload
- `clusterchecks` — Cluster checks metadata
- `haagent` — HA agent metadata
- `host` — Host metadata payload
- `hostgpu` — GPU host metadata
- `hostsysteminfo` — Host system info metadata
- `internal` — Internal metadata utilities
- `inventoryagent` — Agent inventory payload
- `inventorychecks` — Checks inventory payload
- `inventoryhost` — Host inventory payload
- `packagesigning` — Package signing metadata
- `resources` — Resource metadata
- `runner` — Metadata collection runner
- `securityagent` — Security agent metadata
- `systemprobe` — System-probe metadata

## ndmtmp
- `forwarder` — NDM (Network Device Monitoring) temporary forwarder

## netflow
- `common` — Shared NetFlow types
- `config` — NetFlow configuration
- `flowaggregator` — Flow record aggregation
- `format` — Flow record formatting
- `goflowlib` — goflow library integration
- `payload` — NetFlow payload encoding
- `portrollup` — Port rollup logic
- `server` — NetFlow collection server
- `testutil` — Test utilities
- `topn` — Top-N flow tracking

## networkdeviceconfig
- `def` / `fx` / `impl` / `mock` — Network device configuration push

## networkpath
- `npcollector` — Network path collector
- `traceroute` — Traceroute implementation

## notableevents
- `def` / `fx` / `impl` — Notable event emission

## otelcol
- `collector` — OpenTelemetry collector integration
- `collector-contrib` — Contrib OTel components
- `converter` — OTel config converter
- `ddflareextension` — Flare extension for OTel
- `ddprofilingextension` — Profiling extension for OTel
- `logsagentpipeline` — Logs pipeline for OTel
- `otlp` — OTLP ingestion
- `status` — OTel status page

## privateactionrunner
- `def` / `fx` / `impl` — Private action runner for Datadog workflows

## process
- `agent` — Process agent orchestration
- `apiserver` — Process agent API server
- `connectionscheck` — Network connections check
- `containercheck` — Container check
- `expvars` — Expvar endpoint for process agent
- `forwarders` — Process data forwarders
- `gpusubscriber` — GPU metrics subscriber
- `hostinfo` — Host info for process agent
- `processcheck` — Process list check
- `processdiscoverycheck` — Process discovery check
- `profiler` — Process agent profiler
- `rtcontainercheck` — Real-time container check
- `runner` — Check runner for process agent
- `status` — Process agent status
- `submitter` — Payload submitter
- `types` — Shared process types

## publishermetadatacache
- `def` / `fx` / `impl` — Cache for publisher metadata

## rdnsquerier
- `def` / `fx` / `fx-mock` / `fx-none` / `impl` / `impl-none` / `mock` — Reverse DNS querier

## remote-config
- `rcclient` — Remote Configuration client
- `rcservice` — Remote Configuration service
- `rcservicemrf` — MRF (multi-region failover) RC service
- `rcstatus` — Remote Configuration status
- `rctelemetryreporter` — RC telemetry reporter

## serializer
- `logscompression` — Logs payload compression
- `metricscompression` — Metrics payload compression

## snmpscan
- `def` / `fx` / `impl` / `mock` — SNMP device scanning

## snmpscanmanager
- `def` / `fx` / `impl` / `mock` — SNMP scan job management

## snmptraps
- `config` — SNMP trap configuration
- `formatter` — Trap payload formatting
- `forwarder` — Trap event forwarder
- `listener` — UDP trap listener
- `oidresolver` — OID to name resolution
- `packet` — Raw trap packet handling
- `senderhelper` — Metric sender helpers
- `server` — SNMP trap server
- `snmplog` — SNMP trap logging
- `status` — SNMP traps status

## softwareinventory
- `def` / `fx` / `impl` — Software inventory collection

## syntheticstestscheduler
- `common` / `def` / `fx` / `impl` — Synthetics test scheduler

## systray
- `systray` — System tray application (Windows/macOS)

## trace
- `agent` — Trace agent core
- `compression` — Trace payload compression
- `config` — Trace agent configuration
- `etwtracer` — ETW-based tracer (Windows)
- `payload-modifier` — Trace payload modification
- `status` — Trace agent status

## trace-telemetry
- `def` / `fx` / `impl` — Trace agent telemetry

## updater
- `daemonchecker` — Updater daemon health check
- `localapi` — Updater local API server
- `localapiclient` — Updater local API client
- `ssistatus` — SSI (Single Step Instrumentation) status
- `telemetry` — Updater telemetry
- `updater` — Core updater component

## workloadselection
- `def` / `fx` / `impl` — Workload selection rules
