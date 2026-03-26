# pkg/network/netlink

## Purpose

Implements Linux Netlink-based connection tracking (conntrack) for NAT translation. The package is the userspace engine behind the network tracer's ability to resolve the original (pre-NAT) address of a connection — essential for correctly attributing network traffic that passes through DNAT rules (e.g. Docker port mappings, iptables DNAT, Kubernetes services).

**Build constraint:** all files except `circuit_breaker.go` require `//go:build linux`. The package is a no-op on other platforms via `conntrack_noop.go` / `conntracker_noop.go`.

## Key elements

### Core types

| Type | File | Description |
|---|---|---|
| `ConTuple` | `decoding.go` | One half of a conntrack entry: source `netip.AddrPort`, destination `netip.AddrPort`, and protocol byte. |
| `Con` | `decoding.go` | A complete conntrack entry with `Origin ConTuple`, `Reply ConTuple`, and `NetNS uint32`. `IsNAT(c Con) bool` tests whether origin and reply tuples differ (i.e. NAT is active). |
| `Conntracker` | `conntracker.go` | Interface exposing `GetTranslationForConn`, `DeleteTranslation`, `DumpCachedTable`, `Close`, Prometheus `Describe`/`Collect`. Implemented by `realConntracker` (live) and `noOpConntracker` (stub). |
| `Conntrack` | `conntrack.go` | Lower-level interface with a single `Exists(*Con) (bool, error)` method. Used for point lookups, not streaming. |
| `Consumer` | `consumer.go` | Owns the Netlink socket; streams new conntrack events and performs initial table dumps. |
| `Event` | `consumer.go` | Wraps a batch of `netlink.Message` values from one `recvmsg` call, plus the network namespace inode. Must be released via `Done()` to return the buffer to the pool. |
| `Decoder` | `decoding.go` | Turns `Event` objects into `[]Con` slices using a zero-allocation `AttributeScanner`. |
| `Socket` | `socket.go` | Optimised `netlink.Socket` implementation: avoids `MSG_PEEK`, uses a 32 KB pre-allocated receive buffer, and supports BPF filter attachment. |
| `CircuitBreaker` | `circuit_breaker.go` | Rate-limiting guard using an EWMA. Trips when the event rate exceeds `maxEventsPerSec`; causes the Consumer to re-create the socket with a lower BPF sampling rate. |

### Constructor functions

| Function | Description |
|---|---|
| `NewConntracker(cfg, telemetry) (Conntracker, error)` | Creates and starts the full conntracker pipeline (requires `CAP_NET_ADMIN`). Performs an initial dump of the conntrack table for both IPv4 and IPv6, then begins streaming live events. Returns `ErrNotPermitted` when capabilities are absent. |
| `NewConntrack(netNS) (Conntrack, error)` | Creates a point-lookup-only conntrack handle for the given network namespace. |
| `NewConsumer(cfg, telemetry) (*Consumer, error)` | Creates a Consumer without starting the receive loop. |
| `NewNoOpConntracker() Conntracker` | Returns a no-op implementation that always returns `nil` translations. |
| `LoadNfConntrackKernelModule(cfg) error` | Sends a dummy Netlink request that side-effects loading the `nf_conntrack_netlink` kernel module. Called during system-probe startup. |

### BPF sampling (`bpf_sampler.go`)

`GenerateBPFSampler(samplingRate float64) ([]bpf.RawInstruction, error)` generates a cBPF program that randomly drops `(1 - samplingRate)` of incoming Netlink messages using the kernel's `BPF_LD_W_ABS + ExtRand` extension. This is the mechanism used to stay within the configured `conntrack_rate_limit` without dropping the socket.

### Debug helpers (`conntrack_debug.go`)

`DumpCachedTable` and `DumpHostTable` return `map[uint32][]DebugConntrackEntry` (keyed by network namespace inode) in a format matching `conntrack -L` output. Used by the agent flare and the `system-probe debug conntrack` CLI command.

### Telemetry

All hot-path operations (gets, registers, unregisters, evictions) record latency histograms and counters under the `network_tracer__conntracker` Prometheus namespace. The Consumer tracks `enobufs`, `throttles`, `read_errors`, and `msg_errors`.

## Architecture

```
Kernel conntrack table
        |
  Netlink socket (Socket)
        |
   Consumer.receive()
        |  (channel of Events)
   Decoder.DecodeAndReleaseEvent()
        |  ([]Con, only NAT entries)
   conntrackCache (LRU + orphan list)
        |
   realConntracker.GetTranslationForConn()
        |
   network.IPTranslation  →  tracer
```

The cache stores both directions of each NAT entry (`Origin→Reply` and `Reply→Origin`) so lookups can be performed from either side. Entries seen only during streaming (not yet confirmed by a lookup) are marked as **orphans** and garbage-collected after `defaultOrphanTimeout` (2 minutes).

## Usage

The package is consumed by `pkg/network/tracer`:

```go
import "github.com/DataDog/datadog-agent/pkg/network/netlink"

ctr, err := netlink.NewConntracker(cfg, telemetryComp)
if errors.Is(err, netlink.ErrNotPermitted) {
    // fall back to eBPF conntracker or no-op
}

// Later, per connection:
translation := ctr.GetTranslationForConn(connTuple)
if translation != nil {
    // use translation.ReplSrcIP / ReplDstIP
}
```

`pkg/network/tracer/chain_conntracker.go` chains multiple `Conntracker` implementations (eBPF first, then netlink) into a single interface.

### Conntracker selection at startup

The tracer tries conntracker implementations in order:

1. **eBPF conntracker** — preferred; reads from the kernel's conntrack eBPF map directly, avoiding the Netlink socket overhead. Available when `linux_bpf` is compiled in and the kernel supports CO-RE or kprobe loading.
2. **Netlink conntracker** (`NewConntracker`) — fallback; uses this package. Returns `ErrNotPermitted` when `CAP_NET_ADMIN` is absent, in which case the tracer falls back to a no-op.
3. **`noOpConntracker`** — used when conntrack is disabled (`cfg.EnableConntrack = false`) or capabilities are insufficient.

The `Debug` endpoints (`/debug/conntrack/cached`, `/debug/conntrack/host`) call `DumpCachedTable` and `DumpHostTable` from `conntrack_debug.go` and are reachable from the system-probe HTTP API for diagnosing NAT translation issues.

### Multi-namespace support

When `EnableConntrackAllNamespaces` is `true`, the Consumer uses `pkg/util/kernel/netns.GetNetNamespaces` to enumerate all network namespaces visible under `/proc` and opens a separate conntrack socket for each. This is required for Kubernetes node-local DNS and service-mesh environments where each pod has its own network namespace.

## Configuration

Relevant fields from `pkg/network/config.Config`:

| Field | Default | Description |
|---|---|---|
| `ConntrackMaxStateSize` | — | Maximum number of NAT entries held in the LRU cache. |
| `ConntrackRateLimit` | — | Target max Netlink messages/second. Set to `-1` to disable. |
| `ConntrackRateLimitInterval` | — | Tick interval for the circuit breaker EWMA. |
| `ConntrackInitTimeout` | — | Maximum time allowed for the initial table dump. |
| `EnableConntrackAllNamespaces` | `false` | When true, the Consumer dumps conntrack tables for all network namespaces reachable from `/proc`. |

## Platform notes

- Requires Linux kernel 3.15+ for BPF socket filters on Netlink sockets. On older kernels, adaptive sampling is disabled and the agent logs a warning if the rate limit is exceeded.
- Requires `CAP_NET_ADMIN` (checked at runtime via `github.com/syndtr/gocapability`).
- The `no_vsock.go` / `vsock.go` split in `socket/` does not apply here; this package's `Socket` type is a separate, netlink-specific implementation.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network/tracer` | [tracer.md](tracer.md) | Primary consumer. The tracer creates a `Conntracker` via `NewConntracker` and stores it on `Tracer.conntracker`. `GetTranslationForConn` is called for every connection returned by `GetActiveConnections` to fill `ConnectionStats.IPTranslation`. Debug endpoints call `DumpCachedTable` and `DumpHostTable`. |
| `pkg/network` | [network.md](network.md) | Defines `network.IPTranslation` (the `ReplSrcIP`/`ReplDstIP` struct stored on `ConnectionStats`), and `Config.EnableConntrack` / `Config.ConntrackMaxStateSize` that gate this package. |
| `pkg/util/kernel` | [../../pkg/util/kernel.md](../../pkg/util/kernel.md) | `HostVersion()` is consulted to decide whether BPF socket-filter sampling is available (requires kernel ≥ 3.15). The `netns/` sub-package (`GetNetNamespaces`, `WithNS`) is used by the Consumer when `EnableConntrackAllNamespaces` is true to open per-namespace conntrack sockets. |
