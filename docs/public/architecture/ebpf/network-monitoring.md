# Network monitoring (NPM and USM)

-----

Network Performance Monitoring (NPM) tracks every TCP and UDP connection on the host — bytes, retransmits, failures, NAT translations, DNS — and Universal Service Monitoring (USM) parses application-layer protocols (HTTP, HTTP/2, gRPC, Kafka, Postgres, Redis) out of the same traffic, including inside TLS. Both live in the `network_tracer` module of [system-probe](system-probe.md) and share one delivery path: connection data accumulates in a stateful store and is drained per client, either by the process-agent polling `/network_tracer/connections` (see the [process pipeline](../pipelines/processes.md)) or, with direct send, by system-probe forwarding payloads itself. This page covers the tracer flavors, conntrack, DNS, the delta model, and USM internals; how the eBPF objects themselves get loaded (CO-RE, runtime compilation, prebuilt) is on the [system-probe page](system-probe.md#ebpf-loading-strategies).

## Key packages

| Path | Purpose |
|---|---|
| [`cmd/system-probe/modules/network_tracer.go`](<<<SRC>>>/cmd/system-probe/modules/network_tracer.go) | Module factory, `/connections` and debug endpoints |
| [`pkg/network/tracer/tracer.go`](<<<SRC>>>/pkg/network/tracer/tracer.go) | `Tracer`: composes the eBPF tracer, conntracker, reverse DNS, USM monitor, state, gateway lookup, process cache |
| [`pkg/network/tracer/connection`](<<<SRC>>>/pkg/network/tracer/connection) | Connection tracer flavors: `sk/`, `fentry/`, `kprobe/`, `ebpfless_tracer.go` |
| [`pkg/network/tracer/offsetguess`](<<<SRC>>>/pkg/network/tracer/offsetguess) | Struct-offset brute-forcing for the prebuilt kprobe tracer |
| [`pkg/network/tracer/ebpf_conntracker.go`](<<<SRC>>>/pkg/network/tracer/ebpf_conntracker.go) | eBPF conntracker (kprobes on nf_conntrack) |
| [`pkg/network/netlink`](<<<SRC>>>/pkg/network/netlink) | Netlink conntracker fallback and conntrack table dumps |
| [`pkg/network/tracer/cilium_lb.go`](<<<SRC>>>/pkg/network/tracer/cilium_lb.go) | Cilium load-balancer conntracker (reads Cilium's LB maps) |
| [`pkg/network/dns`](<<<SRC>>>/pkg/network/dns) | DNS snooping: socket filter, packet parsing, reverse-DNS cache, per-connection stats |
| [`pkg/network/state.go`](<<<SRC>>>/pkg/network/state.go) | Per-client stateful delta computation (`RegisterClient`, `GetDelta`) |
| [`pkg/network/encoding/marshal`](<<<SRC>>>/pkg/network/encoding/marshal) | `/connections` payload marshaling (protobuf or JSON) |
| [`pkg/network/sender`](<<<SRC>>>/pkg/network/sender) | Direct send: aggregation and forwarding from inside system-probe |
| [`pkg/network/usm`](<<<SRC>>>/pkg/network/usm) | USM monitor, protocol dispatcher, TLS attachment (`ebpf_ssl.go`, `ebpf_gotls.go`, `istio.go`, `nodejs.go`) |
| [`pkg/network/protocols`](<<<SRC>>>/pkg/network/protocols) | Per-protocol eBPF specs and stat keepers: `http/`, `http2/`, `kafka/`, `postgres/`, `redis/`, `tls/`, `events/` |
| [`pkg/network/usm/sharedlibraries`](<<<SRC>>>/pkg/network/usm/sharedlibraries) | eBPF watcher for shared-library loads (feeds TLS uprobe attachment) |
| [`pkg/network/go/bininspect`](<<<SRC>>>/pkg/network/go/bininspect) | Symbol/type inspection of Go binaries for Go-TLS uprobes |
| [`pkg/network/driver`](<<<SRC>>>/pkg/network/driver) | Windows `ddnpm` driver interface |

## Inside the network tracer module

The module factory ([`network_tracer.go`](<<<SRC>>>/cmd/system-probe/modules/network_tracer.go), build tags `(linux && linux_bpf) || (windows && npm) || darwin`) first checks OS/kernel support (`tracer.IsTracerSupportedByOS`, honoring `system_probe_config.excluded_linux_versions`), then builds [`tracer.NewTracer`](<<<SRC>>>/pkg/network/tracer/tracer.go). One `Tracer` owns everything below; NPM and USM are enablement flags on the same object, not separate modules.

```text
 kernel                                   system-probe (network_tracer module)
+-----------------------------+     +-------------------------------------------------+
| tcp_*/udp_* hooks           |     |  ebpfTracer -----> closed conns (perf/ringbuf)  |
| (sk / fentry / kprobe)      +---->|      |                    |                      |
| conn_stats, tcp_stats maps  |     |      | active conns       v                      |
+-----------------------------+     |      |             tcpCloseConsumer              |
| nf_conntrack kprobes        +---->|  conntracker (NAT)        |                      |
+-----------------------------+     |      |                    v      +------------+  |
| DNS socket filter           +---->|  reverseDNS + dns stats   +----> | network    |  |
+-----------------------------+     |                                  | .State     |  |
| protocol dispatcher         +---->|  usmMonitor (protocol stats) --> | (per-client|  |
| socket filter + tail calls  |     |                                  |  deltas)   |  |
+-----------------------------+     |  processCache (event monitor) -> +-----+------+  |
| exec/exit event stream      +---->|                                        |         |
+-----------------------------+     +----------------------------------------+---------+
                                                                             |
                                    GET /network_tracer/connections?client_id=...
                                    (process-agent)      -- or direct send --
```

## Connection tracer flavors

[`newEbpfTracer`](<<<SRC>>>/pkg/network/tracer/connection/ebpf_tracer.go) tries flavors in a fixed order; each flavor returns a sentinel "disabled" error to pass the baton, while a *real* load error aborts module creation (no silent cross-flavor fallback):

1. **SK tracer** ([`sk/`](<<<SRC>>>/pkg/network/tracer/connection/sk)) — the newest flavor, opt-in via `network_config.enable_sk_tracer`. CO-RE-only; built on fentry hooks, `BPF_PROG_TYPE_SOCK_OPS`, socket-local storage, and `task_file` BPF iterators, which require roughly kernel 5.11+ features (each feature is probed at startup). Enabling it force-disables protocol classification, TLS certificate collection, and USM in [`adjust_npm.go`](<<<SRC>>>/pkg/system-probe/config/adjust_npm.go) — they are mutually exclusive today — and requires CO-RE and ring buffers to be enabled.
1. **fentry tracer** ([`fentry/`](<<<SRC>>>/pkg/network/tracer/connection/fentry)) — `network_config.enable_fentry` (default false), CO-RE-only; historically the Fargate-on-kernel-5.10 path. Protocol classification is unsupported on RHEL 9+ with this flavor.
1. **kprobe tracer** ([`kprobe/`](<<<SRC>>>/pkg/network/tracer/connection/kprobe)) — the default workhorse. [`LoadTracer`](<<<SRC>>>/pkg/network/tracer/connection/kprobe/tracer.go) runs the full CO-RE → runtime-compiled → prebuilt fallback chain (see [eBPF loading strategies](system-probe.md#ebpf-loading-strategies)). The CO-RE build needs kernel ≥4.4.128 (any CentOS/RHEL is accepted regardless). The **prebuilt** build uses [offset guessing](<<<SRC>>>/pkg/network/tracer/offsetguess): at startup it brute-forces kernel struct offsets by creating known connections and observing them — visible in tcpdump and occasionally flagged by security tooling. Prebuilt on kernels ≥5.18 cannot track UDPv6, and failed-connection tracking plus TLS certificate collection are disabled on prebuilt. Kprobes attach through the perf ABI, or the legacy `kprobe_events` ABI when `system_probe_config.attach_kprobes_with_kprobe_events_abi` is set.

Two non-eBPF flavors sit outside that chain:

- **ebpfless tracer** ([`ebpfless_tracer.go`](<<<SRC>>>/pkg/network/tracer/connection/ebpfless_tracer.go), `network_config.enable_ebpfless` / `DD_ENABLE_EBPFLESS`): no eBPF at all — AF_PACKET capture on Linux (libpcap on macOS) feeding a userspace TCP state machine. This is the ECS Fargate path and the only path on macOS ([`tracer_darwin.go`](<<<SRC>>>/pkg/network/tracer/tracer_darwin.go)). Protocol classification and process-event enrichment are disabled with it.
- **Windows driver**: connections come from the closed-source `ddnpm` kernel driver via IOCTL/ReadFile ([`tracer_windows.go`](<<<SRC>>>/pkg/network/tracer/tracer_windows.go), [`pkg/network/driver`](<<<SRC>>>/pkg/network/driver)); flows are polled on a fixed interval and fed into the same `network.State`.

The eBPF flavors maintain the same map set — `conn_stats`, `tcp_stats`, `tcp_retransmits`, port-binding maps, `connection_protocol` — sized by `system_probe_config.max_tracked_connections` (default 65536). Closed connections stream through a perf or ring buffer into the `tcpCloseConsumer`; the buffer for closed connections is sized by `system_probe_config.max_closed_connections_buffered` (defaults to `max_tracked_connections`).

## Conntrack and NAT

NAT translation is resolved by a `Conntracker` chosen in [`newConntracker`](<<<SRC>>>/pkg/network/tracer/tracer.go):

1. The **eBPF conntracker** ([`ebpf_conntracker.go`](<<<SRC>>>/pkg/network/tracer/ebpf_conntracker.go), `network_config.enable_ebpf_conntracker`, default true): kprobes on `__nf_conntrack_hash_insert` and friends, with its own CO-RE → runtime-compiled → prebuilt chain; the initial table state is dumped via netlink.
1. Fallback: the **netlink conntracker** ([`pkg/network/netlink`](<<<SRC>>>/pkg/network/netlink)), a sampled `NFNETLINK_CONNTRACK` listener with a rate limit (`system_probe_config.conntrack_rate_limit`, default 500 events/s) and a circuit breaker that shuts sampling down when the kernel floods it.
1. A failure to initialize conntrack is **fatal for the module** unless the error is a permission error or `network_config.ignore_conntrack_init_failure` is set, in which case a no-op conntracker is used and NAT-ed connections show untranslated tuples.

Independently, the **Cilium load-balancer conntracker** ([`cilium_lb.go`](<<<SRC>>>/pkg/network/tracer/cilium_lb.go), `network_config.enable_cilium_lb_conntracker`, default true) reads Cilium's own LB maps to resolve service VIP translations that never hit nf_conntrack; it is chained with the main conntracker via `chainConntrackers`. Translations are attached per connection as it is closed or polled, producing NAT-corrected tuples in the payload. `system_probe_config.enable_conntrack_all_namespaces` widens tracking beyond the root network namespace. Separately, **gateway lookup** ([`gateway_lookup_linux.go`](<<<SRC>>>/pkg/network/gateway_lookup_linux.go), `network_config.enable_gateway_lookup`) resolves the via-gateway and subnet for each connection using a route cache backed by cloud metadata.

## DNS snooping

A dedicated `BPF_PROG_TYPE_SOCKET_FILTER` on a raw socket (a classic cBPF filter instead on the ebpfless path and on pre-4.1.0 kernels) copies DNS packets to userspace, where [`snooper.go`](<<<SRC>>>/pkg/network/dns/snooper.go) parses them into two products: a reverse-DNS cache used to annotate IPs in the connections payload with names, and per-connection DNS query/response stats (`system_probe_config.collect_dns_stats`, `collect_dns_domains`, capped by `max_dns_stats`, default 20000). Monitored ports default to `[53]` (`network_config.dns_monitoring_ports`); `system_probe_config.disable_dns_inspection` turns the whole thing off, and `collect_local_dns` controls whether localhost DNS is kept. On Windows, DNS packets come from the driver instead ([`monitor_windows.go`](<<<SRC>>>/pkg/network/dns/monitor_windows.go)).

## Process and container enrichment

With `event_monitoring_config.network_process.enabled` (kernel ≥4.14), the network tracer registers handlers on [`pkg/network/events`](<<<SRC>>>/pkg/network/events), which the `event_monitor` module feeds with exec/exit events from the shared kernel event stream — this is why the event monitor must load after the network tracer (see [module ordering](system-probe.md#module-registry-and-loader)). A [`processCache`](<<<SRC>>>/pkg/network/tracer/process_cache.go) keyed by PID and start time then stamps closed connections with container IDs and tags, which is what makes per-container network pages work. This enrichment is unavailable with the ebpfless tracer.

## The per-client delta model

`network.State` ([`state.go`](<<<SRC>>>/pkg/network/state.go)) is the heart of delivery. Each consumer registers a `client_id` (`RegisterClient`); on every poll, `GetDelta` returns only the connections closed since that client's last poll plus stat deltas for still-open connections, merging in DNS and USM stats. Multiple clients — the process-agent's connections check, the core agent's USM check, debug endpoints — are fully isolated from one another. Client state expires after two minutes of inactivity (`ClientStateExpiry`, hardcoded in [`pkg/network/config`](<<<SRC>>>/pkg/network/config/config.go)).

The module's HTTP surface reflects this: `GET /network_tracer/connections?client_id=X` returns the delta for `X`, marshaled as `application/protobuf` or JSON depending on the `Accept` header ([`encoding/marshal`](<<<SRC>>>/pkg/network/encoding/marshal)), and `GET /network_tracer/register?client_id=X` pre-registers a client so its first real poll does not return a meaningless "everything" snapshot. The process-agent batches connections into messages of at most `system_probe_config.max_conns_per_message` (default 600) before forwarding — the rest of that path is described in the [process and container pipeline](../pipelines/processes.md).

/// warning
The endpoint is stateful per `client_id`. A client that polls without registering, or two clients sharing an ID, silently get wrong deltas. When testing with `curl`, always pass a throwaway `client_id` so you do not consume the process-agent's delta.
///

## Direct send

With `network_config.direct_send` (Linux-only, `DirectSendSupported`), system-probe stops serving `/connections` entirely — the endpoint and `/register` are not registered — and instead drains state itself: [`pkg/network/sender`](<<<SRC>>>/pkg/network/sender) aggregates connections, tags them using the remote [workloadmeta](../containers/workloadmeta.md)/[tagger](../containers/tagger.md) (fed by a dedicated event-monitor consumer), hands them to the connections forwarder component ([`comp/forwarder/connectionsforwarder`](<<<SRC>>>/comp/forwarder/connectionsforwarder)) for delivery to the connections intake at `process.<site>` (see the [process pipeline](../pipelines/processes.md)), and offers them to the network path collector for path testing. This removes the process-agent from the NPM data path.

## USM internals

USM is enabled by `service_monitoring_config.enabled` and lives inside the same module: `tracer.NewTracer` builds a [`usm.Monitor`](<<<SRC>>>/pkg/network/usm/monitor.go) alongside the connection tracer. [`CheckUSMSupported`](<<<SRC>>>/pkg/network/usm/config/config.go) gates it at kernel ≥4.14; TLS interception on ARM additionally requires kernel ≥5.5 plus runtime compilation or CO-RE. If USM is unsupported and NPM is also enabled, the module continues NPM-only; if USM was the only reason the module existed, creation fails.

### The protocol dispatcher

One eBPF manager ([`ebpf_main.go`](<<<SRC>>>/pkg/network/usm/ebpf_main.go)) anchors everything on a single `BPF_PROG_TYPE_SOCKET_FILTER` program, `socket__protocol_dispatcher`, attached to a raw socket created by [`HeadlessSocketFilter`](<<<SRC>>>/pkg/network/filter/socket_filter.go). It classifies each packet's protocol and **tail-calls** into per-protocol parsing programs through the `protocols_progs` and `tls_process_progs` program arrays. Each protocol is a `protocols.ProtocolSpec` (struct in [`pkg/network/protocols`](<<<SRC>>>/pkg/network/protocols)) declaring its maps, probes, and tail calls, and — via [`buildmode`](<<<SRC>>>/pkg/network/usm/buildmode) — which of CO-RE, runtime compilation, or prebuilt it supports. Parsed transactions stream to userspace through the batching event consumer in [`protocols/events`](<<<SRC>>>/pkg/network/protocols/events), landing in per-protocol stat keepers.

### Protocols and TLS interceptors

The registry (`knownProtocols` in [`ebpf_main.go`](<<<SRC>>>/pkg/network/usm/ebpf_main.go)) contains the wire protocols and the TLS interceptors:

| Spec | Config key (under `service_monitoring_config`) | Notes |
|---|---|---|
| HTTP | `http.enabled` | Request/response pairing, path captured up to `http.max_request_fragment` (≤512 bytes), URL scrubbing via `http.replace_rules` |
| HTTP/2 + gRPC | `http2.enabled` | Kernel ≥5.2; HPACK dynamic-table tracking |
| Kafka | `kafka.enabled` | Produce/fetch request parsing |
| Postgres | `postgres.enabled` | Query classification |
| Redis | `redis.enabled` | Kernel ≥5.4 |
| OpenSSL/GnuTLS | `tls.native.enabled` | Uprobes on libssl/libcrypto/libgnutls |
| Go TLS | `tls.go.enabled` | Uprobes on `crypto/tls` functions in Go binaries |
| Istio/Envoy | `tls.istio.enabled` | Probes the Envoy binary's BoringSSL |
| NodeJS | `tls.nodejs.enabled` | Probes Node's statically linked OpenSSL |

TLS interception works by hooking the plaintext at the library boundary rather than decrypting anything. For native TLS ([`ebpf_ssl.go`](<<<SRC>>>/pkg/network/usm/ebpf_ssl.go)), the [`sharedlibraries`](<<<SRC>>>/pkg/network/usm/sharedlibraries) eBPF program watches `openat`/`openat2` syscalls for shared-library loads and streams matches to userspace, where the shared [`uprobes.UprobeAttacher`](<<<SRC>>>/pkg/ebpf/uprobes/attacher.go) attaches SSL read/write uprobes to each matching library in each process. For Go binaries ([`ebpf_gotls.go`](<<<SRC>>>/pkg/network/usm/ebpf_gotls.go)), [`bininspect`](<<<SRC>>>/pkg/network/go/bininspect) locates the `crypto/tls.(*Conn).Read/Write/Close` symbols and argument locations without needing DWARF, handling Go's ABI differences per version. Traffic decoded by a TLS interceptor is re-dispatched through the same protocol programs, so HTTPS shows up as HTTP stats with a TLS tag.

### The process monitor dependency

TLS attachment needs to know when processes start and stop. USM's process monitor ([`pkg/process/monitor`](<<<SRC>>>/pkg/process/monitor)) can consume netlink proc events, but the preferred source is the event monitor's kernel event stream: with `service_monitoring_config.enable_event_stream` (default when supported), [`eventmonitor_linux.go`](<<<SRC>>>/cmd/system-probe/modules/eventmonitor_linux.go) registers a process-monitor consumer that replays exec/exit events into USM. This is one of the reasons enabling USM often pulls the `event_monitor` module in (see the [enablement matrix](system-probe.md#module-enablement-matrix)).

### Stats delivery

USM stats do not have their own transport: they ride the same `network.State` and appear in the `/connections` payload under `USMData` (HTTP, HTTP2, Kafka, Postgres, Redis), correlated to connections by tuple. The core agent's USM check polls with its own `client_id` and forwards to the intake. Payload capture depth and volume are bounded by `http.max_stats_buffered`, `http.max_tracked_connections`, and `service_monitoring_config.max_concurrent_requests` (defaults to `max_tracked_connections`).

### USM on Windows

[`monitor_windows.go`](<<<SRC>>>/pkg/network/usm/monitor_windows.go) has no eBPF: HTTP transactions are produced by the `ddnpm` driver, and TLS visibility comes from an ETW consumer ([`etw_interface.go`](<<<SRC>>>/pkg/network/protocols/http/etw_interface.go), which also powers IIS-specific enrichment surfaced at `/network_tracer/iis_tags`).

## Kernel version gates

Gates are scattered across [`config_linux_bpf.go`](<<<SRC>>>/pkg/system-probe/config/config_linux_bpf.go), [`usm/config`](<<<SRC>>>/pkg/network/usm/config/config.go), and per-tracer checks; the effective summary:

| Feature | Minimum kernel |
|---|---|
| NPM kprobe tracer (CO-RE) | 4.4.128 (any CentOS/RHEL accepted); Ubuntu 4.4.114–4.4.127 hard-excluded for a kernel panic bug |
| Process event stream (enrichment, USM event stream) | 4.14 |
| USM | 4.14 (TLS on ARM: 5.5, plus runtime compilation or CO-RE) |
| HTTP/2 monitoring | 5.2 |
| Redis monitoring | 5.4 |
| SK tracer | ~5.11 (fentry, sock_ops, sk_storage, task_file iterators — feature-probed) |
| No-prealloc eBPF maps (`disable_map_preallocation`) | 6.1 |
| Prebuilt eBPF | *deprecated* on ≥6.0 (RHEL ≥5.14); prebuilt ≥5.18 loses UDPv6 |

Uretprobes (used by some TLS hooks) are disabled entirely on kernels affected by the uretprobe+seccomp segfault bug.

## Debugging

All endpoints live under `/network_tracer/` on the system-probe socket; pass a throwaway `client_id` where relevant.

| Endpoint | Purpose |
|---|---|
| `/debug/net_maps` | Dump the raw eBPF connection maps as a connections payload |
| `/debug/net_state` | Dump `network.State` internals for a client |
| `/debug/ebpf_maps?maps=m1,m2` | Dump arbitrary registered eBPF maps |
| `/debug/conntrack/cached` | The conntracker's in-memory NAT table |
| `/debug/conntrack/host` | The host's conntrack table via netlink (for comparison) |
| `/debug/process_cache` | The process-enrichment cache |
| `/debug/http_monitoring`, `/debug/http2_monitoring`, `/debug/kafka_monitoring`, `/debug/postgres_monitoring`, `/debug/redis_monitoring` | Live per-protocol USM stats (each returns a note if that protocol is disabled) |
| `/debug/usm_telemetry` | USM internal telemetry |
| `/debug/usm/traced_programs` | Programs currently attached by the USM uprobe attachers |
| `/debug/usm/blocked_processes`, `/debug/usm/clear_blocked` | Binaries USM refused to attach to, and a reset switch |
| `/debug/usm/attach-pid`, `/debug/usm/detach-pid` | Manually force TLS attachment for a PID |
| `/iis_tags`, `/process_cache_tags` | Windows-only: IIS site tags and process-cache tags |

The `/telemetry` and `/debug/stats` global endpoints (see [system-probe](system-probe.md#operating-and-debugging)) carry tracer health: map usage from the eBPF telemetry modifiers, per-module errors, and CO-RE load results.

## Gotchas

- **`enable_sk_tracer` flips other settings**: it force-disables USM, protocol classification, and TLS certificate collection (each logged as a warning). Setting one flag and losing a feature elsewhere is by design, for now.
- **Offset guessing actively creates connections** at startup with the prebuilt kprobe tracer; do not be alarmed by short-lived localhost connections from system-probe in packet captures.
- **A real load error in a newer flavor aborts the module** rather than falling through to kprobes — only the "flavor disabled" sentinel passes the baton. A host where the sk or fentry tracer is enabled but broken gets no NPM at all.
- **Conntrack init failure kills the module** unless it is a permission error or `ignore_conntrack_init_failure` is set; on hosts without conntrack kernel modules this is a common cause of "network tracer failed to start".
- **NPM connection rollups require USM rollups**: `network_config.enable_connection_rollup` is silently disabled if `service_monitoring_config.enable_connection_rollup` is off.
- **USM changes eBPF strategy defaults**: enabling it defaults the runtime compiler and kernel header download to on ([`adjust_usm.go`](<<<SRC>>>/pkg/system-probe/config/adjust_usm.go)).
- **Deltas are per `client_id` and expire**: after two minutes without polls (the hardcoded `ClientStateExpiry`), a client's baseline is dropped and its next poll is a full snapshot again.
- **The ebpfless tracer loses features silently**: protocol classification and process enrichment are turned off with it, which is why Fargate NPM data has no container-process attribution from the tracer side.
- **On Windows, NPM data stopping for 20 minutes makes system-probe exit on purpose** (an inactivity watchdog assuming the process-agent is missing); check the process-agent before suspecting the driver.
