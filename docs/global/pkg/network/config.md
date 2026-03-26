> **TL;DR:** `pkg/network/config` owns all configuration for NPM and USM, reading values from `system-probe.yaml` and exposing a single `*Config` struct that drives every network monitoring subsystem.

# pkg/network/config

## Purpose

`pkg/network/config` owns all configuration for the network tracing subsystem (NPM — Network Performance Monitoring) and USM (Universal Service Monitoring). It reads values from `system-probe.yaml` (via the `system_probe_config`, `network_config`, `service_monitoring_config`, and `event_monitoring_config` namespaces), applies defaults and cross-field validation, and exposes a single `*Config` struct that the rest of the network code imports.

The sub-package `sysctl/` provides a small, caching reader for kernel parameters under `/proc/sys/`, used at startup to align timeouts and buffer sizes with the kernel's actual conntrack settings.

## Key elements

### Key interfaces

This package exposes no interfaces. All consumers interact with `*Config` and `*USMConfig` directly.

### Key types

#### `Config` struct (`config.go`)

`Config` embeds `ebpf.Config` (from `pkg/ebpf`) and adds ~60 network-specific fields. A representative selection:

#### Feature toggles

| Field | Config key | Default |
|-------|-----------|---------|
| `NPMEnabled` | `network_config.enabled` | false |
| `CollectTCPv4Conns` / `CollectTCPv6Conns` | `network_config.collect_tcp_v4/v6` | true |
| `CollectUDPv4Conns` / `CollectUDPv6Conns` | `network_config.collect_udp_v4/v6` | true |
| `DNSInspection` | `!system_probe_config.disable_dns_inspection` | true |
| `CollectDNSStats` | `system_probe_config.collect_dns_stats` | — |
| `CollectDNSDomains` | `system_probe_config.collect_dns_domains` | — |
| `ProtocolClassificationEnabled` | `network_config.enable_protocol_classification` | — |
| `TCPFailedConnectionsEnabled` | `network_config.enable_tcp_failed_connections` | — |
| `EnableNPMConnectionRollup` | `network_config.enable_connection_rollup` | — |
| `EnableEbpfless` | `network_config.enable_ebpfless` | false |
| `EnableFentry` | `network_config.enable_fentry` | false |
| `DirectSend` | `network_config.direct_send` | false (Linux only) |

#### Buffer and capacity limits

| Field | Description |
|-------|-------------|
| `MaxTrackedConnections` | Size of eBPF connection-tracking maps |
| `MaxClosedConnectionsBuffered` | Max closed connections held before flush |
| `MaxFailedConnectionsBuffered` | Max TCP error entries held in memory |
| `MaxConnectionsStateBuffered` | Max per-client state objects |
| `MaxDNSStats` / `MaxDNSStatsBuffered` | DNS stats map size |
| `ClosedChannelSize` / `ClosedBufferWakeupCount` | Perf-buffer tuning for closed-connection events |

#### Timeout and expiry

| Field | Default | Source |
|-------|---------|--------|
| `UDPConnTimeout` | 30 s | `nf_conntrack_udp_timeout` |
| `UDPStreamTimeout` | 120 s | `nf_conntrack_udp_timeout_stream` |
| `TCPConnTimeout` | 2 min | hard-coded |
| `ClientStateExpiry` | 2 min | hard-coded |
| `DNSTimeout` | configurable | `system_probe_config.dns_timeout_in_s` |

#### Conntrack

| Field | Description |
|-------|-------------|
| `EnableConntrack` | Enable NAT tracking via netlink |
| `EnableConntrackAllNamespaces` | Track across all network namespaces |
| `EnableEbpfConntracker` | Use eBPF-based conntracker instead of netlink |
| `EnableCiliumLBConntracker` | Track Cilium load-balancer NAT |
| `ConntrackMaxStateSize` / `ConntrackRateLimit` | Capacity and rate-limiter settings |
| `ConntrackInitTimeout` | Max time to wait for conntrack to initialize |
| `IgnoreConntrackInitFailure` | Don't fail startup on conntrack errors |

#### Traffic filtering

| Field | Description |
|-------|-------------|
| `ExcludedSourceConnections` | Map of IP → port-list to ignore on the source side |
| `ExcludedDestinationConnections` | Same for destination side |
| `DNSMonitoringPortList` | Ports treated as DNS (default `[53]`; HTTP ports 80/443 are rejected with a warning) |
| `RecordedQueryTypes` | DNS record types to collect |

#### Performance flags

| Field | Description |
|-------|-------------|
| `NPMRingbuffersEnabled` | Use eBPF ring buffers instead of perf buffers |
| `CustomBatchingEnabled` | Custom kernel-side batching for perf events |
| `EnableCertCollection` | Collect TLS certificates via uprobes |

#### `USMConfig` struct (`usm_config.go`)

Embedded in `Config` under the `service_monitoring_config` namespace. Contains per-protocol monitoring flags and tuning for HTTP, HTTP/2, Kafka, gRPC, Postgres, Redis, MongoDB, and other L7 protocols. Key fields mirror the NPM pattern: `EnableHTTPMonitoring`, `MaxHTTPStatsBuffered`, `HTTPMapCleanerInterval`, etc.

`NewUSMConfig(cfg)` is called from `New()` and returns a fully populated `*USMConfig`.

### Key functions

#### Constructor

```go
func New() *Config
```

Reads `pkgconfigsetup.SystemProbe()`, calls `sysconfig.Adjust(cfg)` to apply any system-probe-wide adjustments, and returns a fully-populated `*Config`. It also logs a warning for each disabled protocol family.

#### Computed methods

| Method | Description |
|--------|-------------|
| `(*Config).FailedConnectionsSupported() bool` | Returns true only if `TCPFailedConnectionsEnabled` is set and at least one TCP address family is collected. |
| `(*Config).RingBufferSupportedNPM() bool` (Linux) | Returns true if the running kernel supports `BPF_MAP_TYPE_RINGBUF` **and** `NPMRingbuffersEnabled` is set. |

---

### Configuration and build flags

#### `sysctl/` sub-package

Path: `pkg/network/config/sysctl`

A thin, Linux-only library (Darwin stub in `sysctl_darwin.go`) that reads kernel parameters from `/proc/sys/` with optional TTL-based caching.

#### Types

| Type | Description |
|------|-------------|
| `String` | Reads a sysctl as a raw string. |
| `Int` | Reads and parses a sysctl as a single `int`. |
| `IntPair` | Reads a sysctl that contains two space-separated integers (e.g. `nf_conntrack_udp_timeout_stream`). |

All three share the unexported `sctl` base which holds the file path, TTL, last-read timestamp, and a sticky error (permission or not-found errors are remembered and not retried).

#### Constructor pattern

```go
// procRoot is typically "/proc", sysctl is the path relative to /proc/sys/
sysctl.NewInt("/proc", "net/netfilter/nf_conntrack_udp_timeout", 30*time.Second)
```

Pass `0` for `cacheFor` to disable caching.

#### Usage in NPM

`pkg/network/config/netns_linux.go` (and the network tracer startup code) instantiates `sysctl.Int` and `sysctl.IntPair` to read the kernel's conntrack timeout values at startup, using them to populate `Config.UDPConnTimeout` and `Config.UDPStreamTimeout` when the user has not overridden them.

## Usage

### Creating a config

```go
import "github.com/DataDog/datadog-agent/pkg/network/config"

cfg := config.New()
if cfg.NPMEnabled {
    // start the tracer
}
```

`New()` must be called after `pkgconfigsetup.SystemProbe()` has loaded `system-probe.yaml`. This is typically done inside the `system-probe` binary startup sequence.

### Configuration namespaces in `system-probe.yaml`

```yaml
system_probe_config:
  max_tracked_connections: 65536
  collect_dns_stats: true

network_config:
  enabled: true
  collect_tcp_v4: true
  enable_protocol_classification: true
  enable_ebpf_conntracker: true

service_monitoring_config:
  enabled: true
  enable_http_monitoring: true
```

### Checking kernel capability at runtime

```go
if cfg.RingBufferSupportedNPM() {
    // use ring buffers
} else {
    // fall back to perf buffers
}
```
