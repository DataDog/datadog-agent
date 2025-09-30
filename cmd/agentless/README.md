# Agentless Agent

A lightweight, standalone monitoring agent with **full observability capabilities** (traces, metrics, and logs) and embedded remote configuration. Designed for containerized and cloud-native environments where you want comprehensive monitoring without the overhead of the full Datadog Agent.

## ✨ Features

### 🔍 **Full Observability Stack**
- ✅ **APM Traces** - Distributed tracing with remote configuration support
- ✅ **DogStatsD Metrics** - Custom metrics collection via UDP/UDS/Named Pipe
- ✅ **Log Collection** - File tailing, container logs, journald, TCP/UDP listeners, and more
- ✅ **Remote Configuration** - Dynamic configuration updates from Datadog backend

### 🚀 **Performance & Size**
- **6.8 MB compressed** (Linux AMD64 with UPX) vs 100+ MB for standard agent
- **10-15x smaller** than the full Datadog Agent
- **No overhead** - Only includes essential monitoring components
- **Fast startup** - Optimized for containerized environments

### 🔒 **Security & Transport**
- **Unix Domain Sockets (UDS)** by default for traces and metrics
- **Named Pipes** on Windows
- **No network ports** exposed by default (TCP/UDP disabled)
- **Filesystem-based IPC** for better security and performance

### 📦 **Deployment Flexibility**
- **No dependencies** - Single static binary (with CGo for compression)
- **No main agent required** - Fully standalone with embedded remote config
- **Cross-platform** - Linux (AMD64/ARM64), macOS (ARM64), Windows
- **Container-ready** - Designed for Kubernetes, Docker, and cloud environments

## Build

```bash
go build -tags "grpcnotrace serverless zlib zstd" \
  -ldflags "-s -w -X 'github.com/DataDog/datadog-agent/pkg/version.AgentVersion=7.60.0'" \
  -o bin/agentless ./cmd/agentless
```

**Build Tags Explained:**
- `grpcnotrace` - Removes gRPC tracing overhead (reduces binary size)
- `serverless` - Enables serverless-optimized code paths
- `zlib`, `zstd` - Compression for metrics and logs
- ~~`otlp`~~ - **Removed** - OTLP support not needed (saves ~7MB uncompressed, ~1.6MB compressed)

For detailed build instructions including cross-compilation and UPX compression, see [BUILD_AGENTLESS.md](../../BUILD_AGENTLESS.md).

### Binary Sizes

| Platform | Size (UPX compressed) | Size (uncompressed) |
|----------|----------------------|---------------------|
| Linux AMD64 | **6.8 MB** | 26 MB |
| Linux ARM64 | **5.6 MB** | 24 MB |
| macOS ARM64 | **24 MB** (no UPX) | 24 MB |

**Capabilities:**
- ✅ **APM Traces** - Full distributed tracing with remote config
- ✅ **DogStatsD Metrics** - Custom metrics via DogStatsD protocol  
- ✅ **Log Collection** - Multiple sources with compression
- ✅ **Remote Configuration** - Dynamic updates from Datadog
- ❌ **OTLP Protocol** - Not included (saves ~1.6 MB)
- ❌ **Lambda Extensions** - AWS-specific code removed

**Optimizations Applied:**
- Debug symbols stripped (`-s -w` ldflags)
- Lambda-specific code removed (~600 lines)
- OTLP support removed (saves 1.6 MB compressed)
- UPX compression on Linux (73-77% compression ratio)

**Size Overhead:**
- **Metrics + Logs** add only ~0.7 MB (10-11%) vs traces-only build
- **Full agent** is 10-15x smaller than the standard Datadog Agent (~100+ MB)

## Configuration

### Required
- `DD_API_KEY` - Datadog API key (**required**)

### Optional
- `DD_SITE` - Datadog site (default: `datadoghq.com`)
- `DD_LOG_LEVEL` - Log level: `trace`, `debug`, `info`, `warn`, `error` (default: `error`)
- `DD_CONFIG_FILE` - Path to `datadog.yaml` config file (default: `datadog.yaml`)
- `DD_REMOTE_CONFIGURATION_ENABLED` - Enable remote config (default: `true`)

### APM (Traces)
- `DD_APM_ENABLED` - Enable APM trace collection (default: `true`)
- `DD_APM_RECEIVER_PORT` - TCP port for traces (default: `0` - disabled, use socket instead)
- `DD_APM_RECEIVER_SOCKET` - Unix socket path (default: `/tmp/datadog_libagent.socket` on Unix)
- `DD_APM_WINDOWS_PIPE_NAME` - Named pipe (default: `\\.\pipe\datadog-libagent` on Windows)

### Metrics (DogStatsD)
- `DD_DOGSTATSD_PORT` - DogStatsD UDP port (default: `0` - disabled, use socket instead)
- `DD_DOGSTATSD_SOCKET` - Unix socket path (default: `/tmp/datadog_dogstatsd.socket` on Unix)
- `DD_DOGSTATSD_PIPE_NAME` - Named pipe (default: `\\\\.\\pipe\\datadog-dogstatsd` on Windows)

### Logs
- `DD_LOGS_ENABLED` - Enable log collection (default: `false` - **must be explicitly enabled**)
- `DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL` - Collect all container logs (default: `false`)

**Log Sources Supported:**
- 📄 **File tailing** - Tail log files from disk
- 🐳 **Docker containers** - Collect container stdout/stderr
- 📦 **Containerd** - Collect from containerd runtime
- 📓 **Journald** - SystemD journal logs (Linux)
- 🪟 **Windows Event Log** - Windows event viewer logs
- 🌐 **TCP/UDP listeners** - Receive logs over network
- 🔌 **Integrations** - Built-in log sources from integrations

**Log Configuration:**
Logs are configured via `logs_config` in `datadog.yaml` or via integration configs. See example below.
- `DD_CONFIG_FILE` - Path to datadog.yaml configuration file (optional)

### Configuration File

The agent supports configuration via `datadog.yaml` file. Environment variable settings override configuration file values.

Minimal configuration:
```yaml
site: datadoghq.com
log_level: info
remote_configuration:
  enabled: true
apm_config:
  enabled: true
logs_enabled: true  # Enable log collection
logs_config:
  # Example: Tail application log files
  - type: file
    path: /var/log/myapp/*.log
    service: myapp
    source: custom
  
  # Example: Collect from all Docker containers
  container_collect_all: true
  
  # Example: TCP listener for remote logs
  - type: tcp
    port: 10514
    service: syslog
    source: network
```

### Log Configuration Examples

**File Tailing:**
```yaml
logs_config:
  - type: file
    path: /var/log/app/*.log
    service: myapp
    source: python
    tags:
      - env:prod
```

**Container Logs (Docker/Containerd):**
```yaml
logs_config:
  container_collect_all: true  # Collect from all containers
  # Or specific containers:
  - type: docker
    image: nginx
    service: web-proxy
```

**Journald (Linux SystemD):**
```yaml
logs_config:
  - type: journald
    include_units:
      - nginx.service
      - redis.service
```

**TCP/UDP Network Listener:**
```yaml
logs_config:
  - type: tcp
    port: 10514
    service: syslog
```

### Logs Filtering and Scrubbing

The agent supports Datadog's logs filtering and scrubbing features:

```yaml
logs_config:
  processing_rules:
    # Exclude logs matching a pattern
    - type: exclude_at_match
      name: exclude_healthchecks
      pattern: /healthcheck
    
    # Mask sensitive data
    - type: mask_sequences
      name: mask_api_keys
      pattern: api_key=\w+
      replace_placeholder: "api_key=***"
```

Documentation:
- [Global processing rules](https://docs.datadoghq.com/agent/logs/advanced_log_collection/?tab=configurationfile#global-processing-rules)
- [Filter logs](https://docs.datadoghq.com/agent/logs/advanced_log_collection/?tab=configurationfile#filter-logs)

## 📊 Feature Comparison

| Feature | Agentless Agent | Standard Agent | Serverless Agent |
|---------|----------------|----------------|------------------|
| **Binary Size** | 6.8 MB (Linux) | 100+ MB | ~50 MB |
| **APM Traces** | ✅ | ✅ | ✅ |
| **DogStatsD Metrics** | ✅ | ✅ | ✅ |
| **Log Collection** | ✅ All sources | ✅ All sources | ⚠️ Lambda only |
| **Remote Config** | ✅ Embedded | ✅ Via core agent | ✅ Embedded |
| **System Metrics** | ❌ | ✅ | ❌ |
| **Integrations** | ⚠️ Logs only | ✅ Full | ❌ |
| **Process Monitoring** | ❌ | ✅ | ❌ |
| **Network Monitoring** | ❌ | ✅ | ❌ |
| **Security Monitoring** | ❌ | ✅ | ⚠️ Lambda only |
| **Default Transport** | UDS/Named Pipe | TCP/UDP | Lambda API |
| **Use Case** | Containers, VMs | Full monitoring | AWS Lambda only |

## 🎯 Use Cases

### ✅ **Perfect For:**
- **Containerized applications** (Docker, Kubernetes, ECS, etc.)
- **Cloud-native microservices** with APM instrumentation
- **Lightweight VMs** where full agent is too heavy
- **Sidecar containers** in Kubernetes pods
- **Edge deployments** with limited resources
- **Custom application monitoring** (traces + metrics + logs)

### ⚠️ **Not Ideal For:**
- **Infrastructure monitoring** (CPU, memory, disk - use standard agent)
- **Database/service integrations** (Redis, PostgreSQL, etc. - use standard agent)
- **Network performance monitoring** (NPM - requires system-probe)
- **Security/compliance monitoring** (CWS, CSPM - use security-agent)
- **Process monitoring** (Live Processes - use standard agent)

## 🚀 Quick Start

### Docker Example

```bash
docker run -d \
  --name datadog-agentless \
  -e DD_API_KEY=<your-api-key> \
  -e DD_SITE=datadoghq.com \
  -e DD_LOGS_ENABLED=true \
  -e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /var/lib/docker/containers:/var/lib/docker/containers:ro \
  -v /tmp/datadog_libagent.socket:/tmp/datadog_libagent.socket \
  -v /tmp/datadog_dogstatsd.socket:/tmp/datadog_dogstatsd.socket \
  your-registry/datadog-agentless:latest
```

### Kubernetes Sidecar Example

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp
spec:
  containers:
  - name: app
    image: myapp:latest
    env:
    - name: DD_AGENT_HOST
      value: "unix:///tmp/datadog_libagent.socket"
    - name: DD_DOGSTATSD_URL
      value: "unix:///tmp/datadog_dogstatsd.socket"
    volumeMounts:
    - name: datadog-sockets
      mountPath: /tmp
  
  - name: datadog-agentless
    image: your-registry/datadog-agentless:latest
    env:
    - name: DD_API_KEY
      valueFrom:
        secretKeyRef:
          name: datadog
          key: api-key
    - name: DD_LOGS_ENABLED
      value: "true"
    volumeMounts:
    - name: datadog-sockets
      mountPath: /tmp
    - name: varlog
      mountPath: /var/log/pods
      readOnly: true
  
  volumes:
  - name: datadog-sockets
    emptyDir: {}
  - name: varlog
    hostPath:
      path: /var/log/pods
```

### Standalone Binary

```bash
# Download and run
export DD_API_KEY=your_api_key
export DD_LOGS_ENABLED=true
./agentless

# Your application connects via:
# - Traces: /tmp/datadog_libagent.socket (Unix) or \\.\pipe\datadog-libagent (Windows)
# - Metrics: /tmp/datadog_dogstatsd.socket (Unix) or \\.\pipe\datadog-dogstatsd (Windows)
```

## 🔧 Advanced Configuration

### Multi-Environment Setup

```yaml
# datadog.yaml
site: datadoghq.com
tags:
  - env:production
  - team:platform
  - service:api-gateway

apm_config:
  enabled: true
  analyzed_spans:
    api-gateway|http.request: 1.0

logs_enabled: true
logs_config:
  # Application logs
  - type: file
    path: /var/log/app/*.log
    service: api-gateway
    source: nodejs
  
  # Container logs
  container_collect_all: true
  
  # Processing rules
  processing_rules:
    - type: exclude_at_match
      name: exclude_healthchecks
      pattern: /health

# DogStatsD socket path
dogstatsd_socket: /tmp/datadog_dogstatsd.socket
```

## 🔍 Observability Features

### APM (Traces)
- **Distributed tracing** across services
- **Span sampling** and analysis
- **Remote configuration** for sampling rules
- **Trace metrics** generation
- **Custom span tags** and metadata
- **UDS/Named Pipe** transport by default

### Metrics (DogStatsD)
- **Custom metrics** (gauges, counters, histograms, distributions, sets)
- **Service checks** and events
- **Tagging** support
- **Aggregation** before send
- **UDS/Named Pipe** or UDP transport
- **Origin detection** (when using UDS)

### Logs
- **Multiple sources**: Files, containers, journald, TCP/UDP, Windows events
- **Log processing**: Filtering, scrubbing, multi-line aggregation
- **Compression**: zstd/zlib before send
- **Tagging**: Service, source, custom tags
- **Auto-discovery**: Container labels/annotations
- **Structured logging**: JSON parsing support