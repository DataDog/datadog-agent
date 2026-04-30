# Pressure Check — Host-Level PSI (Pressure Stall Information)

## What This Check Does

This check reads Linux Pressure Stall Information from `/proc/pressure/{cpu,memory,io}` and emits host-wide metrics that quantify how much time tasks spend **stalled waiting for resources**. PSI is fundamentally different from utilization: a host at 100% CPU with 5% PSI is healthy (fully utilized, minimal contention), while a host at 40% CPU with 60% PSI has severe scheduling bottlenecks.

### Metrics Emitted

| Metric | Type | Unit | Description |
|---|---|---|---|
| `system.pressure.cpu.some.total` | MonotonicCount | microseconds | Cumulative time at least one task was stalled waiting for CPU |
| `system.pressure.memory.some.total` | MonotonicCount | microseconds | Cumulative time at least one task was stalled on memory (reclaim, swap-in, refaults) |
| `system.pressure.memory.full.total` | MonotonicCount | microseconds | Cumulative time ALL tasks were stalled on memory — indicates thrashing |
| `system.pressure.io.some.total` | MonotonicCount | microseconds | Cumulative time at least one task was stalled on IO |
| `system.pressure.io.full.total` | MonotonicCount | microseconds | Cumulative time ALL tasks were stalled on IO — indicates saturation |

CPU "full" is not emitted because by kernel design it is always zero (there's always at least one task running on CPU when contention exists).

### Requirements

- Linux kernel >= 4.20 (PSI introduced December 2018)
- PSI must be enabled (default in most distributions; can be disabled via `psi=0` boot parameter)
- On kernels that don't support PSI, the check gracefully disables itself at startup with no error spam

### Using the Metrics in Datadog

The raw values are cumulative microseconds. As MonotonicCount, Datadog computes the delta per collection interval automatically.

To get **percentage of wall-clock time spent stalled** (equivalent to the kernel's `avg` values):
```
system.pressure.cpu.some.total / 10000
```

To smooth over longer windows:
```
system.pressure.memory.full.total.rollup(avg, 60) / 10000    # ~60s average
system.pressure.io.some.total.rollup(avg, 300) / 10000       # ~5min average
```

### Interpreting "some" vs "full"

- **"some"**: At least one task was stalled while others continued running. Indicates **added latency** — workloads are slower but the system is still doing productive work.
- **"full"**: ALL non-idle tasks were stalled simultaneously. The CPU was doing kernel housekeeping (reclaim, swapping) but **zero user work progressed**. Any non-trivial "full" value for memory is a serious concern.

---

## What Existed Before This Check

### Container-Level PSI (cgroupv2 only)

The agent already collects per-container PSI from cgroupv2 pressure files, but with significant gaps:

| Source | What's Parsed | What's Emitted | What's Dropped |
|---|---|---|---|
| `<cgroup>/cpu.pressure` | some: Avg10, Avg60, Avg300, Total | `container.cpu.partial_stall` (Rate, from some.Total only) | Avg10, Avg60, Avg300 |
| `<cgroup>/memory.pressure` | some + full: all 4 fields each | `container.memory.partial_stall` (Rate, from some.Total only) | All full data, all avg values |
| `<cgroup>/io.pressure` | some + full: all 4 fields each | `container.io.partial_stall` (Rate, from some.Total only) | All full data, all avg values |

**20 PSI values are parsed per container, but only 3 reach metrics.** The "full" pressure data (memory thrashing, IO saturation) is parsed and then silently dropped at the conversion layer in `pkg/util/containers/metrics/system/collector_linux.go`.

The code path is:
```
cgroupv2 *.pressure files
  → parsePSI() (pkg/util/cgroups/file.go:136)
  → CPUStats / MemoryStats / IOStats (pkg/util/cgroups/stats.go)
  → convertFieldAndUnit() — only PSISome.Total mapped (collector_linux.go:366,391 + collector_disk_linux.go:39)
  → ContainerCPUStats.PartialStallTime / ContainerMemStats.PartialStallTime / ContainerIOStats.PartialStallTime
  → container.{cpu,memory,io}.partial_stall (processor.go:153,174,211)
```

### Host-Level PSI

**Did not exist.** No code read `/proc/pressure/{cpu,memory,io}` before this check. The memory check's `collect_memory_pressure` option reads `/proc/vmstat` for `allocstall_*` and `pgscan_*` counters — those are older memory pressure indicators, not PSI.

---

## What This Check Adds

This check fills the **host-level PSI gap**. It reads directly from `/proc/pressure/*` (procfs, not cgroups) and provides a system-wide view of resource contention.

### Why Host-Level PSI Matters

Container-level PSI tells you *which container* is experiencing pressure. Host-level PSI tells you *whether the system as a whole* is contended — which is critical for:

1. **Infrastructure capacity planning**: Detect when a node is approaching contention limits before containers start degrading. A host with rising `system.pressure.cpu.some.total` needs more CPU or fewer workloads, regardless of which container is feeling it.

2. **Noisy neighbor detection**: When multiple containers share a host, host-level PSI spikes without corresponding per-container spikes indicate **shared resource contention** (kernel locks, page allocator, VFS) that can't be attributed to a single container.

3. **Correlation with utilization**: PSI adds a dimension that utilization metrics miss. High CPU utilization with low PSI = healthy saturation. Moderate CPU utilization with high PSI = lock contention or scheduling pathology. This distinction is invisible with utilization alone.

4. **"Full" pressure visibility**: The existing container metrics only emit "some" pressure. This check adds "full" for memory and IO — the **critical severity signal** indicating total system stalls. Any non-trivial `system.pressure.memory.full.total` rate means the host is thrashing.

5. **Baseline for Kernel Sentinel**: Host-level PSI is the foundational signal for the broader kernel-level observability initiative. Combined with future eBPF-based lock contention and scheduler metrics, it enables root-cause attribution for performance degradation.

---

## Expanding Visibility — Future Opportunities

### Short-term: Fill the Container PSI Gaps

The existing cgroup PSI infrastructure already parses `PSIFull` and `Avg10/60/300` but drops them. Adding these would require:
- New fields in `ContainerMemStats`, `ContainerIOStats` (e.g., `FullStallTime *float64`)
- Additional `convertFieldAndUnit` calls in `collector_linux.go`
- New `sendMetric` calls in `processor.go`

This would add `container.memory.full_stall`, `container.io.full_stall` — per-container "full" pressure that doesn't exist today.

### Medium-term: Kernel Lock Contention (eBPF)

PSI tells you *that* the system is stalled. Lock contention monitoring tells you *why* — which specific kernel locks are causing delays. Using the `contention_begin`/`contention_end` tracepoints (kernel 5.19+) with eBPF in-kernel aggregation, this can be done at <1-3% overhead.

See: `dev/analysis/lock-contention-monitoring-linux-research.md`

### Long-term: Unified Kernel Signals Pipeline

Combining PSI + lock contention + scheduler tracepoints into a single system-probe module enables:
- Automated root-cause attribution ("high IO PSI is caused by contention on `journal_lock` in ext4")
- Predictive degradation detection (rising PSI trend → alert before containers start failing)
- Cross-container interference analysis ("container A's memory reclaim is causing IO stalls in container B via shared page allocator locks")

See: `dev/analysis/kernel-sentinel-strategy-and-codebase-analysis.md`
