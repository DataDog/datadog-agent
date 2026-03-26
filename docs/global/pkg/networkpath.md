> **TL;DR:** `pkg/networkpath` provides the shared types, traceroute runner, payload structures, and metric-sending utilities for the agent's network path tracing feature, modeling hop-by-hop analysis between the agent host and remote destinations.

# pkg/networkpath — Network path tracing

## Purpose

`pkg/networkpath` provides the shared types, configuration, payload structures, and metric-sending utilities for the agent's network path tracing feature. It models traceroute-based hop-by-hop analysis between the agent host and a remote destination, exposing the results as structured `NetworkPath` payloads that are forwarded to the Datadog event platform.

The package is consumed by:

- `pkg/collector/corechecks/networkpath/` — the `network_path` agent check that runs scheduled traceroutes.
- `comp/networkpath/traceroute/` — the component wrapping traceroute execution (system-probe and local variants).
- `pkg/networkpath/telemetry/` — internal telemetry counters.

Actual packet-level traceroute logic lives in the external library `github.com/DataDog/datadog-traceroute`, not in this package.

---

## Key elements

### Key types

#### `traceroute/config/`

Defines the input parameters for a single traceroute execution.

| Symbol | Description |
|--------|-------------|
| `Config` struct | `DestHostname`, `DestPort`, `MaxTTL`, `Timeout`, `Protocol`, `TCPMethod`, `TCPSynParisTracerouteMode`, `ReverseDNS`, `TracerouteQueries`, `E2eQueries`, `DisableWindowsDriver`, plus source service / container metadata. Passed from the check to the traceroute component. |

### Key functions

#### `traceroute/runner/`

The `Runner` wraps the `github.com/DataDog/datadog-traceroute` library and translates its results into `payload.NetworkPath`.

| Symbol | Description |
|--------|-------------|
| `Runner` struct | Holds a `network.GatewayLookup` (for local via-interface info), namespace inode, a memoized network-ID lookup, and the underlying `traceroute.Traceroute` instance. |
| `New(telemetryComp)` | Creates a `Runner`. Gateway lookup failures are non-fatal (a warning is logged). Network ID is fetched from cloud-provider metadata with up to 4 retries. |
| `Runner.Start()` | Warms up the memoized network-ID lookup; call once at startup. |
| `Runner.Run(ctx, config.Config) (payload.NetworkPath, error)` | Executes a traceroute using the configured protocol (TCP/UDP/ICMP), maps the library result to a `payload.NetworkPath`, and enriches it with gateway and network-ID data. Telemetry counters `runs` and `failed_runs` are updated. |
| Constants `DefaultSourcePort`, `DefaultDestPort`, `DefaultNumPaths`, `DefaultMinTTL`, `DefaultDelay` | Defaults used when the calling `Config` leaves fields unset. |

The runner is platform-split: `runner_linux.go` and `runner_nolinux.go` provide OS-specific helpers (e.g. `createGatewayLookup`).

### Key interfaces

#### `payload/`

Defines every type needed to represent and transmit a network-path result.

**Protocol and method types**

| Symbol | Description |
|--------|-------------|
| `Protocol` | String enum: `ProtocolTCP`, `ProtocolUDP`, `ProtocolICMP`. |
| `TCPMethod` | How TCP traceroutes probe the path: `TCPConfigSYN`, `TCPConfigSACK`, `TCPConfigPreferSACK`, `TCPConfigSYNSocket` (Windows). Default: `TCPConfigSYN`. |
| `ICMPMode` | Whether to replace TCP/UDP probes with ICMP: `ICMPModeNone`, `ICMPModeTCP`, `ICMPModeUDP`, `ICMPModeAll`. Default: `ICMPModeNone`. |
| `ICMPMode.ShouldUseICMP(protocol)` | Returns true when ICMP mode overrides the given protocol. |
| `MakeTCPMethod(s)`, `MakeICMPMode(s)` | Case-insensitive constructors from config strings. |

**Path metadata types**

| Symbol | Description |
|--------|-------------|
| `PathOrigin` | Where the path originated: `PathOriginNetworkTraffic`, `PathOriginNetworkPathIntegration`, `PathOriginSynthetics`. |
| `TestRunType` | `TestRunTypeScheduled`, `TestRunTypeDynamic`, `TestRunTypeTriggered`. |
| `SourceProduct` | `SourceProductNetworkPath`, `SourceProductSynthetics`, `SourceProductEndUserDevice`. |
| `CollectorType` | `CollectorTypeAgent`, `CollectorTypeManagedLocation`. |
| `GetSourceProduct(infraMode)` | Returns `SourceProductEndUserDevice` when `infraMode == "end_user_device"`, otherwise `SourceProductNetworkPath`. |

**Result types**

| Symbol | Description |
|--------|-------------|
| `NetworkPath` | Top-level payload: timestamp, agent version, namespace, test IDs, origin, protocol, `Source`, `Destination`, `Traceroute`, `E2eProbe`, and tags. Serialized to JSON and sent to the event platform. |
| `NetworkPathSource` | Source-side metadata: hostname, display name, via-interface (`*payload.Via`), network ID (VPC), service, container ID, public IP. |
| `NetworkPathDestination` | Destination: hostname, port, service. |
| `Traceroute` | Aggregated traceroute results: `Runs []TracerouteRun` and `HopCount HopCountStats` (avg/min/max). |
| `TracerouteRun` | One run: `RunID`, `Source`, `Destination`, and `Hops []TracerouteHop`. |
| `TracerouteHop` | Single hop: `TTL`, `IPAddress`, `ReverseDNS []string`, `RTT float64`, `Reachable bool`. |
| `E2eProbe` | End-to-end probe stats: RTT samples, packet counts, loss percentage, jitter, and RTT latency summary (avg/min/max). |
| `ValidateNetworkPath(path)` | Returns an error if any `TracerouteRun` has an empty destination IP. Called before forwarding to detect incomplete results from system-probe. |

#### `metricsender/`

An abstraction layer for emitting gauge metrics, so the same path-analysis code can run inside the agent check or via StatsD (e.g. in the process agent or standalone tools).

| Symbol | Description |
|--------|-------------|
| `MetricSender` interface | `Gauge(metricName string, value float64, tags []string)`. |
| `NewMetricSenderAgent(sender.Sender)` | Wraps an agent `sender.Sender`; forwards to `sender.Gauge` with an empty host. |
| `NewMetricSenderStatsd(statsd.ClientInterface)` | Wraps a DogStatsD client; forwards to `statsdClient.Gauge` with sampling rate 1. |

### Configuration and build flags

The runner is platform-split: Linux and non-Linux variants are selected by build tags. Configuration is supplied via `config.Config` at call time. Agent-level configuration keys (e.g. `network_path.connections_monitoring.enabled`) are documented in the `## Usage` section below.

---

## Usage

### Running a traceroute from the check

```go
// pkg/collector/corechecks/networkpath/networkpath.go (pattern)
cfg := config.Config{
    DestHostname:      c.config.DestHostname,
    DestPort:          c.config.DestPort,
    MaxTTL:            c.config.MaxTTL,
    Protocol:          c.config.Protocol,
    TCPMethod:         c.config.TCPMethod,
    ReverseDNS:        true,
    TracerouteQueries: c.config.TracerouteQueries,
    E2eQueries:        c.config.E2eQueries,
}
path, err := c.traceroute.Run(context.TODO(), cfg)
if err != nil {
    return fmt.Errorf("failed to trace path: %w", err)
}
if err := payload.ValidateNetworkPath(&path); err != nil {
    return err
}
```

### Sending metrics

```go
metricSender := metricsender.NewMetricSenderAgent(senderInstance)
metricSender.Gauge("datadog.network_path.path.hops", float64(hopCount), tags)
```

### Serializing a path for the event platform

`NetworkPath` is JSON-serialized and forwarded via `comp/forwarder/eventplatform`. The consumer on the Datadog backend reconstructs hops, RTTs, and E2E probe data for visualization.

---

## Platform notes

- TCP SYN traceroute requires raw socket privileges on Linux. On Windows, a kernel driver is used (can be disabled with `DisableWindowsDriver: true`).
- SACK-based traceroute (`TCPConfigSACK` / `TCPConfigPreferSACK`) establishes a real TCP connection to the destination port, providing more accurate path data but requiring the port to be reachable.
- Gateway lookup (`network.GatewayLookup`) enriches the source with the outbound interface; it is optional — if unavailable, `Source.Via` is omitted.
- Network ID (VPC) is fetched from cloud-provider instance metadata on startup with exponential backoff and memoized for the lifetime of the `Runner`.

---

## Related documentation

| Document | Relationship |
|----------|-------------|
| [`pkg/networkdevice`](networkdevice/networkdevice.md) | Sibling NDM library. `pkg/networkpath` uses `pkg/networkdevice/pinger` for ICMP reachability checking and shares the same NDM namespace model. Both packages emit payloads through the agent's event-platform forwarder. |
| [`comp/networkpath/traceroute`](../comp/networkpath/traceroute.md) | Component that wraps `traceroute/runner` behind the `traceroute.Component` interface. Provides both a local (in-process) and a remote (system-probe) implementation. Callers should depend on this component rather than calling `runner.Runner` directly. |
| [`comp/networkpath/npcollector`](../comp/networkpath/npcollector.md) | Higher-level component that continuously monitors live connections and calls `traceroute.Component.Run` for each discovered destination. Consumes `payload.NetworkPath` and `metricsender.MetricSender` from this package. |

### Component hierarchy

```
pkg/networkpath (types, config, runner, metricsender)
  └─ comp/networkpath/traceroute  (component interface + local/remote impls)
       ├─ pkg/collector/corechecks/networkpath  (scheduled network_path check)
       └─ comp/networkpath/npcollector          (traffic-driven continuous probing)
            └─ comp/process/connectionscheck    (feeds observed connections)
```

### Enabling connections monitoring

To enable automatic path tracing for live traffic (driven by `npcollector`), set in `datadog.yaml`:

```yaml
network_path:
  connections_monitoring:
    enabled: true
  collector:
    workers: 4
    pathtest_interval: 60s
    reverse_dns_enrichment:
      enabled: true
```

To enable the on-demand `network_path` check instead, configure it as a standard agent check instance pointing at a specific destination hostname and port.
