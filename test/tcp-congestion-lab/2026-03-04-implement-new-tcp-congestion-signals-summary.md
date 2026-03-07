# TCP Congestion Signals — Implementation Summary

## Overview

Added 23 new per-connection TCP congestion/loss signals to the Datadog agent's CNM eBPF tracer. All signals are collected via 3 new kprobes and existing `tcp_sendmsg`/`tcp_recvmsg` hooks. Supported on CO-RE and runtime-compiled tracers; prebuilt returns zeros. Newer counter fields are guarded by `LINUX_VERSION_CODE` checks for runtime compilation on older kernels (pre-4.6/4.19).

All data lives in separate BPF hash maps (`tcp_congestion_stats`, `tcp_rto_recovery_stats`) to avoid the 512-byte BPF stack constraint in the batch-flush path. 20 of 23 signals piggyback on existing probes with minimal additional overhead; only 3 new kprobes were added.

### Signal aggregation types

Signals use one of four BPF-side aggregation strategies, chosen to produce meaningful
values when read over a 30-second polling interval. All non-counter types are emitted
as `statsd.Gauge()` in DogStatsD — the distinction is in how the BPF side populates them:

- **counter** — monotonically increasing; delta over the interval gives a rate. Always the latest value (which is also the max). Emitted as `statsd.Count()`.
- **gauge (max)** — BPF compares and keeps the highest value across multiple polls per interval. Useful for "worst case" metrics like max in-flight segments or worst congestion state.
- **gauge (min)** — BPF compares and keeps the lowest value across multiple polls per interval. Used for window fields where 0 indicates a zero-window condition; the min catches transient drops. Initialized to UINT32_MAX on first map insert so any real value becomes the min.
- **snapshot** — BPF overwrites on each kprobe event (not polled periodically). Whatever the last `tcp_enter_loss` or `tcp_enter_recovery` wrote is what's reported. Functionally a gauge from DogStatsD's perspective.
- **gauge** — simple point-in-time value (boolean or static per-connection).

---

## Signals by Category

### Platform and kernel version coverage

All new signals are collected on **CO-RE** and **runtime-compiled** tracers only. The
**prebuilt** tracer is deprecated and returns zeros for all new fields. Two of the kept
signals depend on `tcp_sock` fields added in newer kernels: `delivered_ce` and
`reord_seen` require Linux 4.19+. On older kernels these fields are absent and the
signals report 0. The runtime compiler guards these with `LINUX_VERSION_CODE` checks;
CO-RE handles missing fields gracefully via BTF relocations at load time.

### Counters vs gauges

Counter signals (monotonically increasing values where the delta gives a rate) are
generally more useful than gauge signals (point-in-time snapshots of instantaneous state).
Counters answer "how much of X happened?" — directly actionable, easy to alert on, and
aggregate cleanly across time windows. Gauges answer "what was the value at one moment?"
— harder to interpret, sensitive to polling frequency, and can miss transient events
between polls.

### Retransmit Rate / Count

| Signal | Keep | Type | Description |
|--------|:---:|------|-------------|
| `Retransmits` (existing) | **yes** | counter | Retransmitted segment count. Already part of the CNM product via the existing `kprobe/tcp_retransmit_skb`. The primary retransmit signal — directly tells users "how many segments were retransmitted." |
| `max_retrans_out` | no | gauge (max) | Max retransmitted segments still in-flight during interval. **Skip:** a gauge snapshot of how many retransmits were simultaneously unacked. Less useful than the `Retransmits` counter which gives the total. The "max in-flight retransmits" number is hard to interpret or alert on without deep TCP knowledge. |
| `bytes_retrans` | no | counter | Cumulative bytes retransmitted (4.19+). **Skip:** provides the same information as `Retransmits` but measured in bytes instead of segments. The segment count is sufficient for detecting and alerting on retransmission problems |
| `dsack_dups` | no | counter | DSACK-detected spurious retransmits (4.19+). **Skip:** counts retransmissions that were unnecessary — the receiver already had the data. While conceptually interesting (distinguishes real loss from wasted retransmits due to reordering), it's a secondary root-cause signal. `reord_seen` already indicates reordering, and the absolute count of spurious retransmits isn't directly actionable for most users. |

### Loss / Congestion

| Signal | Keep | Type | Description |
|--------|:---:|------|-------------|
| `rto_count` | **yes** | counter | RTO timeout loss events via `kprobe/tcp_enter_loss`. Counts the number of times the kernel's retransmission timeout fired and the connection entered TCP Loss state (ca_state=4). This is the most severe form of packet loss — it means multiple retransmits failed and TCP fell back to exponential backoff. Non-zero `rto_count` is a strong signal of network path problems (black holes, severe congestion, or link failure). Directly actionable: correlate with service latency spikes. |
| `recovery_count` | **yes** | counter | SACK/NewReno fast recovery events via `kprobe/tcp_enter_recovery`. Counts the number of times the connection entered fast recovery (ca_state=3) — a less severe form of loss detection where SACK information allows targeted retransmission without full timeout. Non-zero means packet loss is occurring but the connection is recovering efficiently. Together with `rto_count`, gives a complete picture of loss severity: high `recovery_count` + low `rto_count` = moderate loss handled well; high `rto_count` = severe loss. |
| `max_ca_state` | no | gauge (max) | Worst TCP congestion avoidance state seen during interval (0=Open, 1=Disorder, 2=CWR, 3=Recovery, 4=Loss). CO-RE only. **Skip:** redundant with the counter signals that already cover each meaningful state: state 4 (Loss) = `rto_count > 0`; state 3 (Recovery) = `recovery_count > 0`; state 2 (CWR) = `delivered_ce > 0`; state 1 (Disorder) = duplicate ACKs received, very common and not actionable. The counters are strictly better — they tell you both *that* it happened and *how many times*, work when the connection is blocked (kprobe-based, not poll-dependent), and produce per-interval deltas naturally. As a gauge it also has collection problems: lifetime max means once it hits 4 it stays there forever (stale), per-interval max requires a gauge reset mechanism, and point-in-time snapshot misses transient states between polls. |
| `max_lost_out` | no | gauge (max) | Max SACK/RACK estimated lost segments during interval. **Skip:** a gauge snapshot of how many segments the kernel estimated as lost at one moment. The `rto_count` and `recovery_count` counters are more actionable — they tell you *how often* loss happened, not how many segments were marked lost at peak. |

### Out-of-Order

| Signal | Keep | Type | Description |
|--------|:---:|------|-------------|
| `reord_seen` | **yes** | counter | Reordering events detected by the kernel (4.19+). Non-zero means packets arrived out of order, which causes the receiver to send duplicate ACKs and may trigger spurious retransmissions. Useful for identifying network paths with reordering (common with ECMP/LAG load balancing, wireless networks, or when traffic crosses multiple paths). Directly actionable: persistent reordering suggests a routing or load-balancing issue. |
| `max_sacked_out` | no | gauge (max) | Max segments SACKed by receiver during interval. **Skip:** a gauge snapshot of peak SACK holes. Less useful than `reord_seen` which directly counts reordering events. High `max_sacked_out` can also occur during normal loss recovery (not just reordering), making it ambiguous without additional context. |

### TCP Backlog / In-Flight

| Signal | Keep | Type | Description |
|--------|:---:|------|-------------|
| `max_packets_out` | no | gauge (max) | Max segments in-flight during interval. **Skip:** hard to interpret without knowing the bandwidth-delay product. A high value could mean "healthy high-throughput connection" or "dangerously large window about to collapse." Not actionable on its own. |
| `delivered` | no | counter | Total segments delivered (4.6+). **Skip:** useful as a denominator for computing retransmit rates (`Retransmits / delivered`), but not directly actionable on its own. The absolute segment delivery count doesn't tell users about congestion. Could be reconsidered if we implement computed retransmit-rate metrics. |

### ECN

| Signal | Keep | Type | Description |
|--------|:---:|------|-------------|
| `delivered_ce` | **yes** | counter | Segments delivered with ECN Congestion Experienced (CE) mark (4.19+). Non-zero means routers along the path are signaling congestion *before* dropping packets — an early warning signal that's only available when ECN is negotiated. Directly actionable: rising `delivered_ce` predicts imminent packet loss if congestion isn't relieved. Requires `ecn_negotiated=1` to be meaningful. |
| `ecn_negotiated` | **yes** | gauge | 1 if ECN was successfully negotiated during TCP handshake, 0 otherwise. **Exception to the gauge rule** — this is kept because it's a boolean per-connection property (set once at handshake, never changes). Essential context for interpreting `delivered_ce`: ce=0 + ecn=1 means "no router congestion" (good); ce=0 + ecn=0 means "no visibility into router congestion" (unknown). Scopes `delivered_ce` analysis to the connections where it's meaningful. |

### Zero-Window

| Signal | Keep | Type | Description |
|--------|:---:|------|-------------|
| `probe0_count` | **yes** | counter | Zero-window probe events via `kprobe/tcp_send_probe0`. Counts the number of times the kernel's persist timer fired because the receiver advertised a window of zero (can't accept more data). Non-zero means the receiver is falling behind — application-level backpressure, slow consumer, or undersized socket buffers. Directly actionable: identifies which connections have a slow receiver. The kprobe fires from the kernel timer independently of `tcp_sendmsg`, so it reliably detects zero-window conditions even when the sender is blocked. |
| `min_snd_wnd` | no | gauge (min) | Min peer's advertised window during interval. **Skip:** intended to detect zero-window conditions (min=0), but has a fundamental blind spot — `handle_congestion_stats()` only runs during `tcp_sendmsg`, and when the sender is blocked on a zero-window, `tcp_sendmsg` doesn't fire. The gauge cannot observe the very condition it's designed to detect. `probe0_count` is the reliable signal for zero-window. |
| `min_rcv_wnd` | no | gauge (min) | Min local advertised window during interval. **Skip:** same blind-spot issue as `min_snd_wnd`. Also less actionable — a zero local receive window means the local application isn't reading fast enough, but `probe0_count` on the remote side would be the more direct signal. |

### Loss-Moment Context

These are event-driven snapshots written at the exact moment of loss/recovery, not polled
periodically. They answer "how big was the pipe when it broke?" The last event's values
are what's reported — max/min tracking doesn't apply since each event overwrites the previous.

| Signal | Keep | Type | Description |
|--------|:---:|------|-------------|
| `cwnd_at_last_rto` | no | snapshot | Congestion window when most recent RTO fired. **Skip:** useful for deep TCP debugging but niche — most users don't know what cwnd means or what value to expect. Only meaningful in conjunction with `rto_count > 0`, and even then the absolute cwnd number requires BDP context to interpret. |
| `ssthresh_at_last_rto` | no | snapshot | Slow-start threshold when most recent RTO fired. **Skip:** same reasoning as `cwnd_at_last_rto`. The ssthresh value tells experts "how aggressively did TCP back off" but isn't actionable for most users. |
| `srtt_at_last_rto` | no | snapshot | Smoothed RTT (µs) when most recent RTO fired. **Skip:** answers "was RTT already elevated before timeout?" — useful context but redundant with the existing RTT signal which is already tracked per-connection. |
| `max_consec_rtos` | no | gauge (max) | Peak consecutive RTOs seen (1=minor, 3+=black hole). **Skip:** a severity amplifier for `rto_count` — distinguishes "one timeout then recovery" from "timeout storm / TCP black hole." Interesting but niche; `rto_count` already captures the severity via total count. |
| `cwnd_at_last_recovery` | no | snapshot | Congestion window when most recent fast recovery started. **Skip:** same reasoning as `cwnd_at_last_rto`. |
| `ssthresh_at_last_recovery` | no | snapshot | Slow-start threshold at most recent fast recovery. **Skip:** same reasoning as `ssthresh_at_last_rto`. |
| `srtt_at_last_recovery` | no | snapshot | Smoothed RTT (µs) at most recent fast recovery. **Skip:** same reasoning as `srtt_at_last_rto`. |

### Summary: signals to keep

| Signal | Category | Type | Why |
|--------|----------|------|-----|
| `Retransmits` | Retransmit | counter | Already shipped; primary retransmit signal |
| `rto_count` | Loss | counter | Severe loss events (RTO timeout) |
| `recovery_count` | Loss | counter | Moderate loss events (fast recovery) |
| `reord_seen` | Out-of-Order | counter | Packet reordering events |
| `delivered_ce` | ECN | counter | Router congestion signaling (early warning) |
| `ecn_negotiated` | ECN | gauge | Boolean prerequisite for interpreting `delivered_ce` |
| `probe0_count` | Zero-Window | counter | Receiver backpressure (zero-window probes) |

---

## Tests

### TestTCPCongestionSignals

- **Perturbation method**: `iptablesWrapper` drops all INPUT packets from 127.0.0.1 for 2 seconds, causing the kernel's RTO timer to fire repeatedly (exponential backoff: 200ms, 400ms, 800ms...).
- **Assertions**: `delivered > 0`, `rto_count > 0`, `bytes_retrans > 0`, `cwnd_at_last_rto > 0`, `srtt_at_last_rto > 0`
- **Logging**: All 23 signal values printed via `t.Logf` in `-v` output (two log lines: congestion + loss context)
- **Passes on**: runtime_compiled and CO-RE

### TestTCPZeroWindowProbe

- **Perturbation method**: Server accepts connection but never reads. `TCP_WINDOW_CLAMP` (`setsockopt` option 10) set to 1 on the server socket, forcing the receiver to advertise window=0 almost immediately. Client floods data until write blocks, then waits 3 seconds for zero-window probes to accumulate.
- **Assertions**: `probe0_count > 0`
- **Passes on**: runtime_compiled and CO-RE

---

## Validation Tooling

### DogStatsD Metric Emission

All 23 signals emitted as `network.tcp.congestion.*` DogStatsD metrics per TCP connection
from `GetActiveConnections()`. Tagged with `srcip:<IP>` and `destip:<IP>` (no ports, to
limit cardinality). Handles nil statsd client gracefully (tests pass nil).

**Note:** When multiple connections share the same IP pair, DogStatsD averages gauge
submissions within a flush interval. This causes fractional values for boolean metrics like
`ecn_negotiated` and blurs per-flow accuracy for others. The DogStatsD emission is a
temporary validation tool — the long-term plan is to publish signals per-flow to the
Datadog backend via the agent-payload, which handles per-connection aggregation correctly.

```
# Gauges (statsd.Gauge) — all non-counter signals
network.tcp.congestion.cwnd_at_last_recovery      (gauge/snapshot)
network.tcp.congestion.cwnd_at_last_rto           (gauge/snapshot)
network.tcp.congestion.ecn_negotiated              (gauge, 0 or 1)
network.tcp.congestion.max_ca_state                (gauge/max)
network.tcp.congestion.max_consec_rtos             (gauge/max)
network.tcp.congestion.max_lost_out                (gauge/max)
network.tcp.congestion.max_packets_out             (gauge/max)
network.tcp.congestion.max_retrans_out             (gauge/max)
network.tcp.congestion.max_sacked_out              (gauge/max)
network.tcp.congestion.min_rcv_wnd                 (gauge/min)
network.tcp.congestion.min_snd_wnd                 (gauge/min)
network.tcp.congestion.srtt_at_last_recovery       (gauge/snapshot)
network.tcp.congestion.srtt_at_last_rto            (gauge/snapshot)
network.tcp.congestion.ssthresh_at_last_recovery   (gauge/snapshot)
network.tcp.congestion.ssthresh_at_last_rto        (gauge/snapshot)

# Counts (statsd.Count) — monotonically increasing counters
network.tcp.congestion.bytes_retrans               (count)
network.tcp.congestion.delivered                   (count)
network.tcp.congestion.delivered_ce                (count)
network.tcp.congestion.dsack_dups                  (count)
network.tcp.congestion.probe0_count                (count)
network.tcp.congestion.recovery_count              (count)
network.tcp.congestion.reord_seen                  (count)
network.tcp.congestion.rto_count                   (count)
```

### Docker Compose Lab (`test/tcp-congestion-lab/`)

Two containers on a `172.28.0.0/16` bridge network:

- **tcp-lab-server** (172.28.0.10): iperf3 servers on ports 5201/5202, plus a Python slow-reader on port 9000 (reads 1 byte every 10s, sets `TCP_WINDOW_CLAMP=1` on accepted connections)
- **tcp-lab-client** (172.28.0.20): idle, ready for `docker exec` commands
- Both have `NET_ADMIN` + `SYS_ADMIN` capabilities, and `tcp_ecn=1` set via sysctls at container startup

### Perturbation Scenarios (`perturbations.sh`)

All perturbations are applied via `tc netem` on the client's `eth0` egress (except where noted) and removed after each scenario.

| Scenario | Traffic | Perturbation | Traffic Duration | Perturbation Duration | Signals exercised |
|----------|---------|-------------|:---:|:---:|-------------------|
| `baseline` | `iperf3 -c server -t 30` (single TCP stream) | None | 30s | — | delivered (sanity check) |
| `loss [%]` | `iperf3 -c server -t 30` | `tc netem loss 5%` on client egress (random packet drop) | 30s | 30s | max_lost_out, max_retrans_out, bytes_retrans |
| `heavy-loss` | `iperf3 -c server -t 30` | `tc netem loss 20%` on client egress (high drop rate, triggers RTO) | 30s | 30s | rto_count, max_ca_state=4 |
| `reorder` | `iperf3 -c server -t 30` | `tc netem delay 50ms reorder 25% 50%` on client egress (25% of packets reordered) | 30s | 30s | reord_seen, max_sacked_out |
| `delay` | `iperf3 -c server -t 30` | `tc netem delay 100ms 25ms` on client egress (100ms delay ± 25ms jitter, ~200ms RTT) | 30s | 30s | max_packets_out (more in-flight due to BDP) |
| `wan` | `iperf3 -c server -t 60` | `tc netem delay 50ms 10ms loss 2%` on client egress (realistic WAN) | 60s | 60s | recovery_count, dsack_dups |
| `ecn` | `iperf3 -c server -t 30` (background, with ECN verification via `ss -ti` at +3s) | `tc netem loss 10% ecn` on client egress (marks data packets CE instead of dropping; `tcp_ecn=1` set at container start) | 30s | 30s | delivered_ce |
| `zero-window` | `dd if=/dev/zero bs=4096 count=1000 \| nc -w 60 server 9000` (4MB to slow-reader) | No netem — perturbation is the slow reader itself (`TCP_WINDOW_CLAMP=1`). Receiver advertises window=0, sender sends probes. | background, up to 60s | 30s wait for probes | probe0_count |
| `sack-recovery` | `iperf3 -c server -t 30 -P 4` (4 parallel TCP streams) | `tc netem delay 50ms loss 5% 25%` on client egress (correlated loss + delay) | 30s | 30s | recovery_count, max_sacked_out, max_ca_state≥3 |
| `all` | Runs all 9 above sequentially with 10s sleep between | — | ~5 min total | — | All signals |
| `cleanup` | None | Removes any leftover `tc qdisc` rules on both containers | — | — | — |

Notes:
- All iperf3 traffic targets `172.28.0.10:5201` except `zero-window` which uses port 9000
- `loss` accepts a custom percentage: `./perturbations.sh loss 10`
- The `ecn` scenario prints `ss -ti` output to verify ECN negotiation on the connection

Usage:

```bash
cd test/tcp-congestion-lab
docker compose up -d --build
./perturbations.sh baseline
./perturbations.sh all       # run everything sequentially
./perturbations.sh cleanup   # remove leftover tc rules
```

---

## Platform Support

| Signal | CO-RE fentry | CO-RE kprobe | Runtime | Prebuilt |
|--------|:---:|:---:|:---:|:---:|
| max_packets_out | ✓ | ✓ | ✓ | 0 |
| max_lost_out | ✓ | ✓ | ✓ | 0 |
| max_sacked_out | ✓ | ✓ | ✓ | 0 |
| max_retrans_out | ✓ | ✓ | ✓ | 0 |
| delivered | ✓ | ✓ | ✓ (4.6+) | 0 |
| delivered_ce | ✓ | ✓ | ✓ (4.19+) | 0 |
| bytes_retrans | ✓ | ✓ | ✓ (4.19+) | 0 |
| dsack_dups | ✓ | ✓ | ✓ (4.19+) | 0 |
| reord_seen | ✓ | ✓ | ✓ (4.19+) | 0 |
| max_ca_state | ✓ | ✓ | 0 | 0 |
| rto_count | ✓ | ✓ | ✓ | 0 |
| recovery_count | ✓ | ✓ | ✓ | 0 |
| probe0_count | ✓ | ✓ | ✓ | 0 |

---

## Known Limitations

- **`max_ca_state` on runtime**: CO-RE only due to `__builtin_preserve_field_info` for bitfield reads. Runtime tracer reports 0. TODO in code for manual bpf_probe_read_kernel approach.
- **`TCPMaxCAState` struct placement**: Between uint32 fields, causing 3 bytes padding. TODO in code to move to trailing byte section.
- **Kernel version guards**: Counter fields `delivered` (4.6+), `delivered_ce`, `bytes_retrans`, `dsack_dups`, `reord_seen` (all 4.19+) are guarded by `LINUX_VERSION_CODE` for runtime compiler. CO-RE handles via BTF relocations. Older kernels report 0.
- **DogStatsD emission**: `emitCongestionMetrics()` in `tracer.go` is temporary for validation. Tagged with `srcip:` and `destip:` (no ports).
- **Debug sleep**: 30s `time.Sleep` in `TestTCPCongestionSignals` for manual bpftool inspection. TODO to remove.
- **Unit test gaps**: `recovery_count`, `reord_seen`, `delivered_ce` not yet validated in unit tests (each requires specific network conditions).

Future phases (Phase 3: loss-moment context + window/RTT/ECN fields, Phase 4: BPF_SOCK_OPS)
are documented in the plan file (`~/.claude/plans/refactored-sniffing-whale.md`).
