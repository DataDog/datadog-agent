# Use-case index

"I want to do X — which package(s) should I look at?"

---

## Checks and metrics

| Goal | Packages |
|---|---|
| Write a new Go check | `pkg/collector/check`, `pkg/collector/corechecks` |
| Emit a metric from a check | `pkg/aggregator/sender` |
| Emit an event or service check | `pkg/aggregator/sender`, `pkg/metrics/event`, `pkg/metrics/servicecheck` |
| Schedule a check | `pkg/collector/scheduler`, `pkg/collector/runner` |
| Load a Python check | `pkg/collector/python`, `pkg/collector/loaders` |
| Load a shared-library (Rust) check | `pkg/collector/sharedlibrary` |
| Add a check that requires system-probe | `pkg/system-probe`, `pkg/system-probe/api` |
| Persist check state across restarts | `pkg/persistentcache` |
| Access check metadata (inventory) | `comp/metadata/inventorychecks` |

## Configuration

| Goal | Packages |
|---|---|
| Read a config value | `pkg/config/model`, `comp/core/config` |
| Register a new config key with defaults | `pkg/config/setup` |
| Detect current platform / container runtime | `pkg/config/env` |
| Read config from another agent process | `pkg/config/fetcher` |
| Get config at runtime without fx | `pkg/config/setup` (`Datadog()` singleton) |
| Subscribe to config changes | `pkg/config/model` (`OnUpdate`) |
| Remote Configuration (RC) product | `pkg/config/remote`, `pkg/remoteconfig/state`, `comp/remote-config/rcclient` |

## Logging

| Goal | Packages |
|---|---|
| Log a message | `pkg/util/log` |
| Initialize the logger | `pkg/util/log/setup`, `comp/core/log` |
| Redirect a third-party logger (zap, klog) | `pkg/util/log/zap`, `pkg/util/log` |
| Rate-limit noisy log sites | `pkg/util/log` (`NewLogLimit`) |

## Telemetry and observability

| Goal | Packages |
|---|---|
| Register a Prometheus metric | `pkg/telemetry`, `comp/core/telemetry` |
| Expose expvar values | `comp/agent/expvarserver` |
| Profile the agent process | `pkg/util/profiling`, `comp/core/profiler` |
| Dump goroutine stacks | `pkg/util/goroutinesdump` |
| Add to the agent status page | `comp/core/status` |
| Add to a flare | `comp/core/flare` |

## Data pipeline (metrics → Datadog)

| Goal | Packages |
|---|---|
| Understand the metrics pipeline | `pkg/aggregator`, `pkg/aggregator/sender`, `pkg/serializer`, `comp/aggregator/demultiplexer` |
| Forward metrics/events to Datadog | `comp/forwarder/defaultforwarder` |
| Forward Event Platform events | `comp/forwarder/eventplatform` |
| Compress a payload | `pkg/util/compression`, `comp/serializer/metricscompression` |
| Serialize metrics to JSON/protobuf | `pkg/serializer`, `pkg/metrics` |

## Log collection

| Goal | Packages |
|---|---|
| Understand the logs pipeline | `pkg/logs` |
| Add a new log source launcher | `pkg/logs/launchers` |
| Add a new log tailer | `pkg/logs/tailers` |
| Work with log messages | `pkg/logs/message` |
| Add a log processing rule | `pkg/logs/processor` |
| Add a log scheduler | `pkg/logs/schedulers` |

## APM / tracing

| Goal | Packages |
|---|---|
| Understand the trace pipeline | `pkg/trace` |
| Add trace sampling logic | `pkg/trace/sampler` |
| Add a span transformation | `pkg/trace/transform`, `pkg/trace/traceutil` |
| Ingest OTLP spans | `pkg/trace/otel`, `comp/otelcol/otlp` |
| Obfuscate SQL/Redis queries | `pkg/obfuscate` |

## eBPF and kernel

| Goal | Packages |
|---|---|
| Write an eBPF program | `pkg/ebpf`, `pkg/ebpf/bytecode` |
| Attach a uprobe | `pkg/ebpf/uprobes` |
| Access eBPF maps from Go | `pkg/ebpf/maps` |
| Detect kernel version / features | `pkg/util/kernel` |
| Read network namespaces | `pkg/util/kernel` (`netns/`) |
| Instrument a Go binary (DI/live debugger) | `pkg/dyninst` |

## Network monitoring (NPM/USM)

| Goal | Packages |
|---|---|
| Understand NPM connection tracking | `pkg/network`, `pkg/network/tracer` |
| Add a USM protocol | `pkg/network/protocols`, `pkg/network/usm` |
| Track DNS queries | `pkg/network/dns` |
| Inspect Go binaries for TLS tracing | `pkg/network/go` |
| Decode NPM payloads | `pkg/network/encoding` |

## Security (CWS/CSPM)

| Goal | Packages |
|---|---|
| Understand CWS architecture | `pkg/security` |
| Write a SECL rule | `pkg/security/secl`, `pkg/security/secl/rules` |
| Add a kernel event type | `pkg/security/secl/model`, `pkg/security/probe` |
| Add a CWS resolver | `pkg/security/resolvers` |
| Build a security profile | `pkg/security/security_profile` |
| Run compliance benchmarks | `pkg/compliance` |

## Kubernetes / cluster

| Goal | Packages |
|---|---|
| Query the Kubernetes API server | `pkg/util/kubernetes/apiserver` |
| Get pod/node metadata | `pkg/util/kubernetes`, `pkg/util/kubelet` |
| Build a cluster-agent feature | `pkg/clusteragent` |
| Add an admission webhook | `pkg/clusteragent/admission` |
| Embed kube-state-metrics | `pkg/kubestatemetrics` |
| Dispatch cluster checks | `pkg/clusteragent/clusterchecks` |

## Containers and workloads

| Goal | Packages |
|---|---|
| Get container metadata | `comp/core/workloadmeta` |
| Tag entities | `comp/core/tagger`, `pkg/tagger` |
| Autodiscover checks/logs from containers | `comp/core/autodiscovery` |
| Query containerd / Docker / CRI-O | `pkg/util/containerd`, `pkg/util/docker`, `pkg/util/crio` |
| Read cgroup stats | `pkg/util/cgroups` |
| Generate SBOM | `pkg/sbom`, `pkg/util/trivy` |

## Cloud providers

| Goal | Packages |
|---|---|
| Detect cloud provider | `pkg/util/cloudproviders` |
| Get EC2 instance metadata | `pkg/util/ec2` |
| Get ECS task metadata | `pkg/util/ecs` |
| Get AWS credentials | `pkg/util/aws` |
| Detect Fargate | `pkg/util/fargate` |

## Hostname and tagging

| Goal | Packages |
|---|---|
| Resolve the agent hostname | `pkg/util/hostname`, `comp/core/hostname` |
| Collect host tags | `pkg/hosttags` |
| Format/merge tag slices | `pkg/util/tags` |
| Efficient tag sets (aggregator hot path) | `pkg/tagset` |

## DogStatsD

| Goal | Packages |
|---|---|
| Understand DogStatsD server | `comp/dogstatsd/server` |
| Add a listener (UDP/UDS/named pipe) | `comp/dogstatsd/listeners` |
| Add metric name mapping | `comp/dogstatsd/mapper` |
| Capture/replay DogStatsD traffic | `comp/dogstatsd/replay` |

## IPC and APIs

| Goal | Packages |
|---|---|
| Add an HTTP endpoint to the agent API | `comp/api/api`, `pkg/api` |
| Add a gRPC service | `comp/api/grpcserver`, `pkg/util/grpc` |
| Call the agent API from a CLI command | `comp/core/ipc` |

## fx / dependency injection

| Goal | Packages |
|---|---|
| Wire a new fx component | `pkg/util/fxutil` |
| Define an optional dependency | `pkg/util/option` |
| Understand the component architecture | `pkg/util/fxutil`, `comp/core/config` |

## Windows-specific

| Goal | Packages |
|---|---|
| Access Windows Performance counters (PDH) | `pkg/util/pdhutil` |
| Read Windows Event Logs | `pkg/util/winutil` (`eventlog/`) |
| ETW event tracing | `comp/etw` |
| Windows kernel driver (NPM, APM inject) | `pkg/windowsdriver` |
| Windows service management | `pkg/util/winutil` (`servicemain/`) |

## Fleet automation / updater

| Goal | Packages |
|---|---|
| Understand the Fleet updater | `pkg/fleet`, `pkg/fleet/installer` |
| Add a new managed package | `pkg/fleet/installer` (`packages/`) |
| Remote-trigger a task | `pkg/fleet/daemon`, `comp/updater/updater` |

## Utilities (misc)

| Goal | Packages |
|---|---|
| HTTP client with proxy/TLS | `pkg/util/http` |
| Retry with backoff | `pkg/util/retry`, `pkg/util/backoff` |
| Scrub credentials from strings | `pkg/util/scrubber` |
| Filesystem utilities | `pkg/util/filesystem` |
| Parse ELF binaries safely | `pkg/util/safeelf` |
| String interning (high-perf) | `pkg/util/intern` |
| Generic map/slice helpers | `pkg/util/maps`, `pkg/util/slices` |
| Quantile estimation (DDSketch) | `pkg/util/quantile` |
