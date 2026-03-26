> **TL;DR:** `pkg/network/tracer` is the top-level NPM connection tracer that orchestrates eBPF programs, DNS snooping, NAT translation, gateway lookup, and USM to observe, enrich, and expose TCP/UDP connections from the kernel.

# pkg/network/tracer

## Purpose

`pkg/network/tracer` is the top-level package for the Network Performance Monitoring (NPM) connection tracer. It orchestrates all subsystems needed to observe, enrich, and expose network connections from the kernel:

- Attaches eBPF programs (or, optionally, a network-tap-based eBPF-less agent) to observe TCP/UDP connections.
- Looks up NAT translations via conntrack so connections are reported with their pre-NAT addresses.
- Resolves gateway information for cloud environments.
- Enriches connections with process and container metadata.
- Feeds DNS snooping and USM (Universal Service Monitoring) data into the connection records.
- Enforces user-configured source/destination connection filters.

The package is only built under the `linux_bpf` build tag (Linux eBPF) or `windows && npm` (Windows NPM). The Windows path is in `tracer_windows.go`; the Linux path is in `tracer.go`.

---

## Key Elements

### Key types

#### `Tracer` (tracer.go)

The central type. Owns references to every subsystem:

| Field | Type | Role |
|---|---|---|
| `ebpfTracer` | `connection.Tracer` | Reads connections from eBPF maps |
| `state` | `network.State` | Per-client delta / closed-connection buffer |
| `conntracker` | `netlink.Conntracker` | NAT translation (eBPF or netlink) |
| `reverseDNS` | `dns.ReverseDNS` | IP-to-hostname resolution |
| `usmMonitor` | `*usm.Monitor` | Protocol-level stats (HTTP, gRPC, Kafka, etc.) |
| `gwLookup` | `network.GatewayLookup` | Gateway resolution for cloud instances |
| `processCache` | `*processCache` | PID → tags/container-ID mapping |
| `containerStore` | `*containers.ContainerStore` | resolv.conf per container |
| `connectionProtocolMapCleaner` | `*ddebpf.MapCleaner` | Periodic cleanup of the eBPF protocol map |

**Lifecycle:**

```
NewTracer(cfg, telemetry, statsd) → *Tracer
    ↳ newTracer()        — creates and wires all subsystems
    ↳ tr.start()         — calls ebpfTracer.Start() and reverseDNS.Start()

Tracer.Stop()            — tears down in reverse order
Tracer.Pause() / Resume()— bypasses eBPF programs without stopping
```

**Primary API used by system-probe:**

- `GetActiveConnections(clientID string) (*network.Connections, func(), error)` — returns the delta since the last call for the given client. Flushes pending closed connections, merges USM stats, resolves IPs to hostnames, and attaches telemetry.
- `RegisterClient(clientID string)` — registers a new consumer of connection deltas.
- `DebugNetworkState`, `DebugNetworkMaps`, `DebugEBPFMaps`, `DebugCachedConntrack`, `DebugHostConntrack`, `DebugDumpProcessCache` — inspection endpoints exposed via the system-probe HTTP API.

**Connection expiry:** TCP connections are kept alive in the userspace state as long as they appear in conntrack (to handle long-lived idle connections). UDP connections expire based on the kernel sysctl `nf_conntrack_udp_timeout`.

---

### Key interfaces

#### `connection/` sub-package — eBPF backend abstraction

Defines the `Tracer` interface and its three implementations:

```go
type Tracer interface {
    Start(func(*network.ConnectionStats)) error
    Stop()
    GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error
    FlushPending()
    Remove(conn *network.ConnectionStats) error
    GetMap(string) (*ebpf.Map, error)
    DumpMaps(w io.Writer, maps ...string) error
    Type() TracerType
    Pause() error
    Resume() error
    // prometheus.Collector
}
```

**`TracerType` constants:**

| Constant | Description |
|---|---|
| `TracerTypeKProbePrebuilt` | Prebuilt (CO-RE-free) kprobe binary |
| `TracerTypeKProbeRuntimeCompiled` | kprobe binary compiled at runtime |
| `TracerTypeKProbeCORE` | CO-RE kprobe binary |
| `TracerTypeFentry` | fentry/fexit programs (preferred when supported) |
| `TracerTypeEbpfless` | eBPF-less network tap backend |

`connection.NewTracer(cfg, telemetry)` selects the right implementation: eBPF-less if `cfg.EnableEbpfless` is set, otherwise `newEbpfTracer`.

**`connection/kprobe/`** — kprobe manager. Loads the eBPF asset (CORE → runtime-compiled → prebuilt), runs `offsetguess.TracerOffsets.Offsets()` to discover struct offsets before first use, and handles protocol classification probes.

**`connection/fentry/`** — fentry manager. Used when BTF and fentry/fexit attachment are supported (generally kernel ≥ 5.5 or newer).

**`connection/ebpfless/`** — eBPF-less tracer. Replaces eBPF with a userspace network tap. Used in environments where eBPF is unavailable (e.g., some container runtimes, older kernels). Processes TCP state via `tcp_processor.go`.

**`connection/batch_extractor.go` / `perf_batching.go`** — batch-reads closed-connection events from the perf ring buffer, reducing overhead for high-connection-rate workloads.

---

### Key functions

#### `offsetguess/` sub-package — kernel struct offset discovery

Because the kprobe tracer reads fields out of kernel structs (`sock`, `inet_sock`, `flowi4/6`, `sk_buff`, conntrack tuples) whose offsets vary across kernel versions, `offsetguess` discovers the right offsets at startup by:

1. Creating real TCP/UDP connections.
2. Running a small eBPF program that samples field values.
3. Walking through `GuessWhat` targets one by one (source/dest address, ports, network namespace, RTT, etc.) until all are confirmed.

**Key types:**

| Type | Description |
|---|---|
| `GuessWhat` | Enum of the fields to guess (`GuessSAddr`, `GuessDAddr`, `GuessRTT`, `GuessNetNS`, `GuessCtTupleOrigin`, …) |
| `State` | Offset-guessing state machine: `StateUninitialized` → `StateChecking` → `StateChecked` → `StateReady` |
| `TracerStatus` | Mirror of the C `tracer_status_t` struct shared with the eBPF program |

The entry point used by the kprobe backend is `offsetguess.TracerOffsets.Offsets(cfg, m)`.

---

#### `networkfilter/` sub-package — connection exclusion filters

Translates the user configuration keys `network_config.excluded_source_connections` / `excluded_destination_connections` into `ConnectionFilter` structs and evaluates connections against them.

**Key types and functions:**

| Symbol | Description |
|---|---|
| `ConnectionFilter` | Holds a CIDR prefix and a set of per-port protocol flags |
| `ConnTypeFilter` | `{TCP bool, UDP bool}` — which transport protocols a rule applies to |
| `FilterableConnection` | Minimal adapter (type + source/dest `netip.AddrPort`) |
| `ParseConnectionFilters(map[string][]string)` | Parses config into `[]*ConnectionFilter`; supports IP, CIDR, IPv4, IPv6, wildcards |
| `IsExcludedConnection(src, dst []*ConnectionFilter, conn)` | Returns `true` if the connection matches any filter |

Port rules support ranges (`"80-443"`), protocol prefixes (`"TCP 443"`, `"UDP *"`), and wildcards (`"*"`).

---

## Usage

The package is consumed exclusively by `cmd/system-probe` via the `NetworkTracer` module factory:

```go
// cmd/system-probe/modules/network_tracer_linux.go
var NetworkTracer = &module.Factory{
    Name:      config.NetworkTracerModule,
    Fn:        createNetworkTracerModule,
    NeedsEBPF: tracer.NeedsEBPF,
}
```

The module creates a `*tracer.Tracer`, registers it with the system-probe HTTP router, and exposes:

- `GET /connections?client_id=<id>` — calls `GetActiveConnections`
- `POST /register_client` — calls `RegisterClient`
- Various `/debug/*` endpoints

Internally, `tracer.go` is the only file that wires together all sub-packages. Tests use `newTracer()` directly (without `start()`) and inject mock or test-mode variants of the sub-systems.

### Configuration and build flags

| Tag | Meaning |
|---|---|
| `linux_bpf` | Full Linux eBPF build; required for most functionality |
| `windows && npm` | Windows NPM build; uses `tracer_windows.go` |

Neither tag is set by default. The `linux_bpf` tag is added by the build system when compiling system-probe on Linux.

---

## Data flow

```
eBPF kprobes / fentry / eBPF-less tap
        |
        v
connection.Tracer.GetConnections()     [connection/ sub-package]
        |
        v
Tracer.GetActiveConnections()
        +--> netlink.Conntracker.GetTranslationForConn()   [NAT translation]
        +--> network.State.GetDelta()                       [delta + USM merge]
        |       +--> dns.ReverseDNS.GetDNSStats()
        |       +--> usm.Monitor.GetProtocolStats()
        +--> dns.ReverseDNS.Resolve()                       [IP→hostname]
        +--> GatewayLookup.LookupWithIPs()
        |
        v
*network.Connections  -->  encoding/marshal  -->  system-probe HTTP response
```

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network` | [network.md](network.md) | Defines the core types (`ConnectionStats`, `Connections`, `State`) that `Tracer` produces and manages. |
| `pkg/network/config` | [config.md](config.md) | `*config.Config` is the sole constructor argument. Every subsystem inside the tracer reads its settings from this struct. |
| `pkg/network/dns` | [dns.md](dns.md) | The tracer creates a `dns.ReverseDNS` via `newReverseDNS(cfg, ...)` and calls `Resolve` + `GetDNSStats` on every `GetActiveConnections` call. |
| `pkg/network/netlink` | [netlink.md](netlink.md) | Provides `Conntracker`. The tracer chains eBPF and netlink conntrack implementations via `chain_conntracker.go` and applies the resulting `IPTranslation` to each `ConnectionStats`. |
| `pkg/network/usm` | [usm.md](usm.md) | The tracer creates and starts a `*usm.Monitor`. Calls `monitor.GetProtocolStats()` inside `GetActiveConnections` so USM data is merged into the connections delta. |
| `pkg/network/tracer/offsetguess` | — | Discovers kernel struct field offsets at startup before kprobe programs are loaded. Only used on the kprobe backend path. |
| `pkg/ebpf` | [../../pkg/ebpf.md](../../pkg/ebpf.md) | Supplies `ebpf.Config`, `MapCleaner`, CO-RE/RC/prebuilt program loading, and `UprobeAttacher` used by the connection sub-packages and USM. |
