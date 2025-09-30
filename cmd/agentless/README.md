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
- `DD_LOGS_CONFIG_LOGS` - **NEW!** Define log sources via JSON (files, TCP/UDP, journald, Windows events)

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

## 📨 DogStatsD Protocol Reference

### Payload Format

The payload format is **identical** for UDP, UDS, and Named Pipes - only the transport changes.

**Basic Format:**
```
<metric_name>:<value>|<type>|@<sample_rate>|#<tags>|T<timestamp>
```

**Components:**
- `<metric_name>` - Metric name (e.g., `my_app.request_count`)
- `<value>` - Numeric value (or multiple values separated by `:`)
- `<type>` - Metric type: `c` (counter), `g` (gauge), `h` (histogram), `d` (distribution), `ms` (timing), `s` (set)
- `@<sample_rate>` - Optional: Sample rate (e.g., `@0.5` = 50%)
- `#<tags>` - Optional: Tags (e.g., `#env:prod,region:us-east-1`)
- `T<timestamp>` - Optional: Unix timestamp

### Examples

**Counter:**
```
page.views:1|c
page.views:1|c|#route:/home,method:GET
```

**Gauge:**
```
temperature:72.5|g
memory.usage:1024|g|#host:web01
```

**Histogram:**
```
request.duration:250|h
request.duration:250|h|@0.5|#endpoint:/api
```

**Distribution:**
```
response.size:512|d|#status:200
```

**Timing:**
```
query.time:45|ms|#db:postgres
```

**Set:**
```
unique.visitors:user123|s
```

**Multi-value (Protocol 1.1):**
```
latency:10:20:30:40|h|#service:api
```

**Batching (multiple metrics):**
```
page.views:1|c|#route:/home
active.users:42|g
request.duration:250|h|#endpoint:/api
```

### Sending Metrics via Unix Domain Socket

**Python:**
```python
import socket

sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
sock.connect('/tmp/datadog_dogstatsd.socket')

# Single metric
sock.send(b'page.views:1|c|#env:prod')

# Batched metrics
sock.send(b'''page.views:1|c|#route:/home
active.users:42|g
request.duration:250|h|#endpoint:/api''')

sock.close()
```

**Go:**
```go
conn, _ := net.Dial("unixgram", "/tmp/datadog_dogstatsd.socket")
defer conn.Close()

conn.Write([]byte("page.views:1|c|#env:prod"))
```

**Node.js:**
```javascript
const dgram = require('dgram');
const client = dgram.createSocket({ type: 'unix_dgram' });

client.connect('/tmp/datadog_dogstatsd.socket', () => {
    client.send('page.views:1|c|#env:prod');
    client.close();
});
```

**Shell (netcat/socat):**
```bash
# Using netcat
echo "page.views:1|c|#env:prod" | nc -U /tmp/datadog_dogstatsd.socket

# Using socat
echo "temperature:72.5|g" | socat - UNIX-CONNECT:/tmp/datadog_dogstatsd.socket
```

### Sending Metrics via Named Pipe (Windows)

**PowerShell:**
```powershell
$pipe = New-Object System.IO.Pipes.NamedPipeClientStream(".", "datadog-dogstatsd", [System.IO.Pipes.PipeDirection]::Out)
$pipe.Connect()
$writer = New-Object System.IO.StreamWriter($pipe)
$writer.WriteLine("page.views:1|c|#env:prod")
$writer.Flush()
$pipe.Close()
```

### Service Checks

**Format:**
```
_sc|<name>|<status>|d:<timestamp>|h:<hostname>|#<tags>|m:<message>
```

**Status codes:** `0` (OK), `1` (WARNING), `2` (CRITICAL), `3` (UNKNOWN)

**Example:**
```
_sc|my_service.check|0|#env:prod|m:Service is healthy
```

### Events

**Format:**
```
_e{<title_length>,<text_length>}:<title>|<text>|d:<timestamp>|h:<hostname>|p:<priority>|t:<alert_type>|#<tags>
```

**Example:**
```
_e{10,15}:Deployment|Deploy finished|#env:prod
```

### Transport Comparison

| Feature | UDP | Unix Domain Socket | Named Pipe (Windows) |
|---------|-----|-------------------|----------------------|
| **Payload Format** | Same | Same | Same |
| **Origin Detection** | ❌ | ✅ Process/Container ID | ✅ Process ID |
| **Performance** | Network overhead | Faster (no network) | Faster (no network) |
| **Security** | Network accessible | Filesystem permissions | OS permissions |
| **Packet Loss** | Possible | Rare | Rare |

### Origin Detection (UDS/Named Pipe Only)

When using UDS or Named Pipes, the agent automatically detects:
- **Process ID** of the sender
- **Container ID** (if in container)
- **Pod name** (if in Kubernetes)

This enables **automatic tagging** without manual configuration!

## 📝 Logs Configuration via Environment Variables

### Basic Logs Setup

**Enable logs:**
```bash
export DD_LOGS_ENABLED=true
```

### Container Logs (Docker/Containerd/Podman)

**Collect all container logs:**
```bash
export DD_LOGS_ENABLED=true
export DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true
```

**Docker-specific:**
```bash
# Use file-based collection (default: true)
export DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE=true

# Force file collection even with existing registry
export DD_LOGS_CONFIG_DOCKER_CONTAINER_FORCE_USE_FILE=true

# Custom docker data root path
export DD_LOGS_CONFIG_DOCKER_PATH_OVERRIDE=/custom/docker/path

# Docker client timeout (seconds)
export DD_LOGS_CONFIG_DOCKER_CLIENT_READ_TIMEOUT=30
```

**Kubernetes-specific:**
```bash
# Use file-based collection
export DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE=true

# Or use kubelet API
export DD_LOGS_CONFIG_K8S_CONTAINER_USE_KUBELET_API=true

# Validate pod container ID
export DD_LOGS_CONFIG_VALIDATE_POD_CONTAINER_ID=true

# Kubelet API timeout
export DD_LOGS_CONFIG_KUBELET_API_CLIENT_READ_TIMEOUT=30s
```

**Podman:**
```bash
export DD_LOGS_CONFIG_USE_PODMAN_LOGS=true
```

### Log Processing

**Global processing rules (JSON format):**
```bash
# Exclude healthcheck logs
export DD_LOGS_CONFIG_PROCESSING_RULES='[{"type":"exclude_at_match","name":"exclude_healthchecks","pattern":"/health"}]'

# Mask sensitive data
export DD_LOGS_CONFIG_PROCESSING_RULES='[{"type":"mask_sequences","name":"mask_api_keys","pattern":"api_key=\\w+","replace_placeholder":"api_key=***"}]'

# Multiple rules (JSON array)
export DD_LOGS_CONFIG_PROCESSING_RULES='[
  {"type":"exclude_at_match","name":"exclude_healthchecks","pattern":"/health"},
  {"type":"mask_sequences","name":"mask_passwords","pattern":"password=\\w+","replace_placeholder":"password=***"}
]'
```

**Auto multi-line detection:**
```bash
export DD_LOGS_CONFIG_AUTO_MULTI_LINE_DETECTION=true
```

**Tag truncated logs:**
```bash
export DD_LOGS_CONFIG_TAG_TRUNCATED_LOGS=true
```

### Performance Tuning

**File limits:**
```bash
# Maximum number of files to tail simultaneously
export DD_LOGS_CONFIG_OPEN_FILES_LIMIT=500

# File scan period (seconds)
export DD_LOGS_CONFIG_FILE_SCAN_PERIOD=10
```

**Buffer sizes:**
```bash
# UDP frame size (bytes)
export DD_LOGS_CONFIG_FRAME_SIZE=9000

# Max message size (bytes)
export DD_LOGS_CONFIG_MAX_MESSAGE_SIZE_BYTES=1000000
```

**Pipeline settings:**
```bash
# Number of log pipelines (defaults to min(4, CPU cores))
export DD_LOGS_CONFIG_PIPELINES=4

# Aggregation timeout (ms)
export DD_LOGS_CONFIG_AGGREGATION_TIMEOUT=1000

# Close timeout for rotated files (seconds)
export DD_LOGS_CONFIG_CLOSE_TIMEOUT=60
```

### Transport & Compression

**Protocol:**
```bash
# Force HTTP (recommended)
export DD_LOGS_CONFIG_FORCE_USE_HTTP=true

# HTTP protocol version (auto/http1/http2)
export DD_LOGS_CONFIG_HTTP_PROTOCOL=auto

# HTTP timeout (seconds)
export DD_LOGS_CONFIG_HTTP_TIMEOUT=10

# Use port 443
export DD_LOGS_CONFIG_USE_PORT_443=true
```

**Compression:**
```bash
# Enable compression (default: true)
export DD_LOGS_CONFIG_USE_COMPRESSION=true

# Compression algorithm (zstd/gzip/deflate)
export DD_LOGS_CONFIG_COMPRESSION_KIND=zstd

# Zstd compression level (1-22, default: 3)
export DD_LOGS_CONFIG_ZSTD_COMPRESSION_LEVEL=3

# Gzip compression level (1-9, default: 6)
export DD_LOGS_CONFIG_COMPRESSION_LEVEL=6
```

**Batching:**
```bash
# Batch wait time (seconds)
export DD_LOGS_CONFIG_BATCH_WAIT=5

# Max batch content size (bytes)
export DD_LOGS_CONFIG_BATCH_MAX_CONTENT_SIZE=5000000

# Max concurrent sends
export DD_LOGS_CONFIG_BATCH_MAX_CONCURRENT_SEND=10
```

### Advanced Options

**Proxy:**
```bash
# SOCKS5 proxy
export DD_LOGS_CONFIG_SOCKS5_PROXY_ADDRESS=localhost:1080

# Custom logs endpoint
export DD_LOGS_CONFIG_LOGS_DD_URL=custom-logs-endpoint.example.com:10516

# Disable SSL (only with proxy)
export DD_LOGS_CONFIG_LOGS_NO_SSL=true
```

**Tagging:**
```bash
# Add source_host tag to TCP/UDP logs
export DD_LOGS_CONFIG_USE_SOURCEHOST_TAG=true

# Tagger warmup duration (seconds)
export DD_LOGS_CONFIG_TAGGER_WARMUP_DURATION=0
```

**Auditing:**
```bash
# Auditor TTL in hours (registry cleanup)
export DD_LOGS_CONFIG_AUDITOR_TTL=23
```

### Complete Example: Container Logs with Processing

```bash
#!/bin/bash

# Enable logs
export DD_LOGS_ENABLED=true

# Collect all container logs
export DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true

# Use file-based collection for reliability
export DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE=true
export DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE=true

# Auto-detect multi-line logs
export DD_LOGS_CONFIG_AUTO_MULTI_LINE_DETECTION=true

# Processing rules (exclude healthchecks and mask secrets)
export DD_LOGS_CONFIG_PROCESSING_RULES='[
  {"type":"exclude_at_match","name":"exclude_health","pattern":"GET /health"},
  {"type":"exclude_at_match","name":"exclude_metrics","pattern":"GET /metrics"},
  {"type":"mask_sequences","name":"mask_tokens","pattern":"token=[A-Za-z0-9]+","replace_placeholder":"token=***"}
]'

# Performance tuning
export DD_LOGS_CONFIG_OPEN_FILES_LIMIT=1000
export DD_LOGS_CONFIG_PIPELINES=4

# Use HTTP with compression
export DD_LOGS_CONFIG_FORCE_USE_HTTP=true
export DD_LOGS_CONFIG_COMPRESSION_KIND=zstd
export DD_LOGS_CONFIG_ZSTD_COMPRESSION_LEVEL=3

# Start the agent
./bin/agentless
```

### File-based Log Collection (Manual)

**Note:** For file-based logs, you need to mount the log directory and use labels/annotations:

**Docker:**
```bash
docker run -d \
  -e DD_LOGS_ENABLED=true \
  -e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true \
  -v /var/log/myapp:/var/log/myapp:ro \
  -v /var/lib/docker/containers:/var/lib/docker/containers:ro \
  your-registry/datadog-agentless
```

**Kubernetes (via annotations):**
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp
  annotations:
    ad.datadoghq.com/myapp.logs: '[{"source":"nodejs","service":"myapp"}]'
spec:
  containers:
  - name: myapp
    image: myapp:latest
```

### Defining Log Sources via Environment Variables

✅ **NEW:** You can now define log sources directly via environment variables using JSON format!

**File tailing:**
```bash
export DD_LOGS_CONFIG_LOGS='[
  {
    "type": "file",
    "path": "/var/log/myapp/*.log",
    "service": "myapp",
    "source": "python",
    "tags": ["env:prod", "team:backend"]
  }
]'
```

**Multiple sources:**
```bash
export DD_LOGS_CONFIG_LOGS='[
  {
    "type": "file",
    "path": "/var/log/nginx/access.log",
    "service": "nginx",
    "source": "nginx"
  },
  {
    "type": "file",
    "path": "/var/log/app/*.log",
    "service": "myapp",
    "source": "python"
  },
  {
    "type": "tcp",
    "port": 10514,
    "service": "syslog",
    "source": "network"
  }
]'
```

**TCP/UDP network listener:**
```bash
export DD_LOGS_CONFIG_LOGS='[
  {
    "type": "tcp",
    "port": 10514,
    "service": "syslog",
    "source": "network"
  }
]'
```

**Journald (Linux):**
```bash
export DD_LOGS_CONFIG_LOGS='[
  {
    "type": "journald",
    "include_units": ["nginx.service", "docker.service"],
    "service": "system",
    "source": "journald"
  }
]'
```

**Windows Event Log:**
```bash
export DD_LOGS_CONFIG_LOGS='[
  {
    "type": "windows_event",
    "channel_path": "Application",
    "service": "windows",
    "source": "eventlog"
  }
]'
```

**Complete example with all options:**
```bash
export DD_LOGS_CONFIG_LOGS='[
  {
    "type": "file",
    "path": "/var/log/app/*.log",
    "exclude_paths": ["/var/log/app/*.tmp"],
    "service": "myapp",
    "source": "python",
    "tags": ["env:prod"],
    "start_position": "end",
    "encoding": "utf-8",
    "log_processing_rules": [
      {
        "type": "exclude_at_match",
        "name": "exclude_debug",
        "pattern": "DEBUG"
      }
    ]
  }
]'
```

### Log Source Configuration Options

| Field | Type | Description | Examples |
|-------|------|-------------|----------|
| `type` | string | Source type | `file`, `tcp`, `udp`, `journald`, `windows_event`, `docker`, `containerd` |
| `path` | string | File path (supports wildcards) | `/var/log/app/*.log` |
| `exclude_paths` | array | Paths to exclude | `["/var/log/app/*.tmp"]` |
| `service` | string | Service name tag | `myapp`, `nginx` |
| `source` | string | Source name tag | `python`, `nginx`, `custom` |
| `tags` | array | Custom tags | `["env:prod", "region:us-east"]` |
| `start_position` | string | Where to start tailing | `beginning`, `end` (default) |
| `encoding` | string | File encoding | `utf-8`, `utf-16-be`, `shift-jis` |
| `port` | int | TCP/UDP port | `10514` |
| `include_units` | array | Journald units to include | `["nginx.service"]` |
| `channel_path` | string | Windows event channel | `Application`, `Security` |
| `log_processing_rules` | array | Per-source processing rules | See processing rules format |

### Alternative Methods

You can still use:

1. ✅ **Environment variable** (DD_LOGS_CONFIG_LOGS) - **NEW! Works for everything**
2. ✅ **Container autodiscovery** (labels/annotations) - Best for containers
3. ✅ **Configuration file** (`datadog.yaml` or integration configs) - Traditional method

All three methods can be used simultaneously!