> **TL;DR:** `pkg/network/dns` captures DNS packets off the wire, builds an IP-to-hostname reverse-DNS cache, and tracks per-connection DNS statistics (latency, timeouts, response codes) for the NPM tracer.

# pkg/network/dns

## Purpose

`pkg/network/dns` provides DNS query monitoring and IP-to-hostname reverse resolution for the NPM connection tracer. It:

- Captures DNS packets off the wire (UDP and TCP/port 53) using a raw AF_PACKET socket.
- Parses query/response pairs to build an IP → hostname cache.
- Tracks per-connection DNS statistics: latency, timeouts, and response codes.
- Exposes a `ReverseDNS` interface consumed by `pkg/network/tracer` to annotate connections with DNS names.

The package is built under `linux_bpf` or `windows && npm`. On Linux the raw socket is optionally filtered by an eBPF `SOCKET_FILTER` program; on older kernels (< 4.1.0) or in eBPF-less mode it falls back to a classic BPF filter. On Windows a separate driver-based packet source is used (`driver_windows.go`, `packet_source_windows.go`).

---

## Key Elements

### Key interfaces

#### `ReverseDNS` interface (types.go)

The public contract consumed by the tracer:

```go
type ReverseDNS interface {
    Resolve(map[util.Address]struct{}) map[util.Address][]Hostname
    GetDNSStats() StatsByKeyByNameByType
    WaitForDomain(domain string) error   // used in tests
    Start() error
    Close()
}
```

Two implementations:

| Implementation | Description |
|---|---|
| `*dnsMonitor` (`monitor_linux.go`) | Full implementation backed by `socketFilterSnooper` + optional eBPF program |
| `nullReverseDNS` (`null.go`) | No-op used when `cfg.DNSInspection` is false |

**Constructor:** `dns.NewReverseDNS(cfg, telemetry)` — called by `tracer.newReverseDNS`.

---

### Key types

| Type | Description |
|---|---|
| `Hostname` | Interned `*intern.StringValue`; use `ToString(h)`, `ToHostname(s)`, `HostnameFromBytes(b)` for conversions |
| `QueryType` | DNS record type constant (`TypeA`, `TypeAAAA`, `TypeCNAME`, `TypeMX`, `TypeSRV`, etc.) |
| `Key` | Connection identifier for DNS stats: `{ServerIP, ClientIP, ClientPort, Protocol}` |
| `Stats` | Per-domain statistics: `Timeouts uint32`, `SuccessLatencySum uint64`, `FailureLatencySum uint64`, `CountByRcode map[uint32]uint32` |
| `StatsByKeyByNameByType` | `map[Key]map[Hostname]map[QueryType]Stats` — the full stats payload returned by `GetDNSStats()` |

---

### Key functions

#### `socketFilterSnooper` (snooper.go)

The internal implementation of the live packet capture loop.

```go
type socketFilterSnooper struct {
    source     filter.PacketSource  // AF_PACKET raw socket
    parser     *dnsParser           // gopacket-based DNS decoder
    cache      *reverseDNSCache     // IP → []Hostname, capacity 100 000, 1-min expiry
    statKeeper *dnsStatKeeper       // optional per-connection DNS stats
    ...
}
```

**Key methods:**

| Method | Description |
|---|---|
| `Resolve(ips)` | Looks up IPs in the in-memory cache |
| `GetDNSStats()` | Drains and resets all accumulated stats from `dnsStatKeeper` |
| `processPacket(data, info, ts)` | Parses a single captured packet; updates cache and stat keeper |
| `Close()` | Shuts down the packet capture goroutine, closes the socket and cache |

Two goroutines are started at construction time: `pollPackets` (packet capture loop) and `logDNSStats` (10-minute periodic log of query/success/error counts).

---

#### `dnsParser` (parser.go)

A `gopacket`-based parser supporting Ethernet/IPv4/IPv6/UDP/TCP stacks. TCP DNS uses a custom `tcpWithDNSSupport` layer to handle message framing (2-byte length prefix per RFC 1035 §4.2.2). Responses are decoded into a reusable `translation` object (IP → hostname mapping with timestamps) to avoid per-packet allocations.

Errors surfaced:

| Error | Meaning |
|---|---|
| `errTruncated` | Packet was cut short |
| `errSkippedPayload` | Valid packet but no DNS content of interest |

---

#### `reverseDNSCache` (cache.go)

An in-memory LRU-like map of `util.Address → *dnsCacheVal`. Cache entries are bounded by a fixed size (100,000 entries) and expire after 1 minute. Each IP can map to at most 1,000 domain names; oversized mappings are logged and counted.

Telemetry counters (under the `network_tracer__dns_cache` module): `size`, `lookups`, `hits`, `added`, `expired`, `oversized`.

---

#### `dnsStatKeeper` (stats.go)

Tracks in-flight queries by transaction ID and computes latency once the matching response arrives. Stats are partitioned by `Key × Hostname × QueryType`. The entire state is drained atomically on `GetAndResetAllStats()` to provide a consistent snapshot to the tracer state machine.

Configuration parameters:

| Config field | Effect |
|---|---|
| `cfg.CollectDNSStats` | Enables `dnsStatKeeper`; without it, only the reverse-DNS cache is maintained |
| `cfg.CollectDNSDomains` | Includes domain names in stats (vs. just connection keys) |
| `cfg.DNSTimeout` | Duration after which an unmatched query is counted as a timeout |
| `cfg.MaxDNSStats` | Maximum number of stat objects retained; older entries are evicted |
| `cfg.CollectLocalDNS` | If false, loopback DNS traffic is excluded from stats |

---

### Configuration and build flags

#### eBPF program (bpf.go / ebpf.go)

On Linux with kernel ≥ 4.1.0 and eBPF enabled, an `ebpfProgram` wraps an `ebpf-manager` instance that loads a `SOCKET_FILTER` program (`socket__dns_filter`). This program filters DNS packets in kernel space before they are delivered to the AF_PACKET socket, reducing CPU cost for hosts with high non-DNS traffic. On older kernels or in eBPF-less mode the filter is replaced with a classic BPF filter generated in `generateBPFFilter`.

---

#### Telemetry

Snooper-level counters (under `network_tracer__dns`):

| Counter | Description |
|---|---|
| `decoding_errors` | Packets that failed to decode |
| `truncated_pkts` | Truncated packets |
| `queries` | DNS queries seen |
| `successes` | Successful DNS responses |
| `errors` | Failed DNS responses (NXDOMAIN, SERVFAIL, etc.) |

---

## Usage

`pkg/network/tracer` is the sole consumer:

```go
// tracer.go
tr.reverseDNS = newReverseDNS(cfg, telemetryComponent)
// ...
tr.reverseDNS.Start()

// During GetActiveConnections:
conns.DNS = t.reverseDNS.Resolve(ips)            // hostname resolution
delta := t.state.GetDelta(..., t.reverseDNS.GetDNSStats(), ...)  // stats
```

The resolved `map[util.Address][]Hostname` is attached directly to the `*network.Connections` payload sent to the process-agent and ultimately to the Datadog backend.

DNS monitoring is enabled by `cfg.DNSInspection` (config key: `network_config.enable_dns_inspection`). When disabled, `newReverseDNS` returns a `nullReverseDNS` that no-ops all calls.

### DNS data flow in the wider pipeline

```
Raw packets (AF_PACKET socket / Windows driver)
        |
  socketFilterSnooper.processPacket()
        |
   dnsParser  (gopacket, Ethernet/IP/UDP/TCP)
        |
   reverseDNSCache (IP → []Hostname, 100k entries, 1-min TTL)
   dnsStatKeeper   (per connection × hostname × QueryType)
        |
   ReverseDNS.Resolve() + GetDNSStats()
        |
   tracer.GetActiveConnections()
        |
   network.State.GetDelta()   ← dnsStats merged here
        |
   network.Connections.DNS    ← attached to payload
        |
   encoding/marshal.FormatConnection()   ← encoded by dnsFormatter
        |
   system-probe HTTP response (protobuf)  →  process-agent
```

The `dnsFormatter` inside `pkg/network/encoding/marshal` converts `StatsByKeyByNameByType` into protobuf `model.DNSStats` entries attached to each connection in the serialised payload. See [encoding.md](encoding.md) for details of the serialisation layer.

### Build tags

| Tag | Meaning |
|---|---|
| `linux_bpf` | Full Linux implementation with eBPF filter support |
| `windows && npm` | Windows implementation using driver-based packet source |
| neither | Only `null.go` (no-op) is compiled |

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network/tracer` | [tracer.md](tracer.md) | The sole production consumer. Creates `ReverseDNS` via `newReverseDNS`, calls `Start()`, `Resolve()`, and `GetDNSStats()` on every `GetActiveConnections` call. |
| `pkg/network` | [network.md](network.md) | Defines `network.Connections.DNS` (`map[util.Address][]Hostname`) and `network.State.GetDelta`, which receives the `StatsByKeyByNameByType` from `GetDNSStats()` and merges it into the connection delta. |
| `pkg/network/encoding` | [encoding.md](encoding.md) | `marshal.ConnectionsModeler` creates a `dnsFormatter` that converts `StatsByKeyByNameByType` into protobuf `model.DNSStats` per connection during `FormatConnection`. |
| `pkg/ebpf` | [../../pkg/ebpf.md](../../pkg/ebpf.md) | On Linux ≥ 4.1.0, an `ebpfProgram` wraps a `pkg/ebpf.Manager` to load the `SOCKET_FILTER` program (`socket__dns_filter`). The eBPF program reduces CPU cost by filtering non-DNS packets in kernel space before delivery to the AF_PACKET socket. |
