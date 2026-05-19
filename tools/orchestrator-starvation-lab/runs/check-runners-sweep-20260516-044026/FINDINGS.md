# DD_CHECK_RUNNERS sweep — GIL knee under parse-bound load

## Setup

- **Noise**: 200 openmetrics cluster-checks, each scraping a Python HTTP server
  that returns a pre-built ~647 KB Prometheus payload (5,000 metric series with
  realistic labels) in ~3 ms. Per-check work is dominated by Python-side
  parsing in the openmetrics integration — **GIL-held**.
- **CLCRs**: 3 replicas, fixed.
- **Total noise WorkersNeeded** (theoretical, ignoring GIL): ~67 (200 checks ×
  3s exec / 15s interval) — i.e. you’d “need” 67 workers across the fleet to
  scrape every check on time.
- **Sweep**: `DD_CHECK_RUNNERS` ∈ {1, 2, 4, 8, 16, 32, 64}, each cell
  rolled out via operator + 3-minute settle, then `agent status` sampled
  across all CLCRs.

## Headline numbers

```
N_per_pod  total_workers  samples  exec_p50  exec_p95  exec_max  exec_mean  rpm/check
    1           3            201    0.533s    0.621s    0.721s    0.535s     1.182
    2           6            114    1.200s    2.043s    2.570s    1.305s     0.944
    4          12            201    2.331s    2.827s    3.175s    2.310s     1.040
    8          24            201    4.752s    5.735s    6.604s    4.690s     0.961
   16          48            201   10.365s   12.975s   16.105s    9.880s     0.949
   32          96            195   21.590s   25.612s   31.827s   21.159s     0.808
   64         192            179   32.480s   43.209s   51.734s   30.064s     1.014
```

## Interpretation

Per-check `exec_mean` roughly **doubles with every doubling of N**:

```
N → 2N      mean ratio
1 → 2       2.44×
2 → 4       1.77×
4 → 8       2.03×
8 → 16      2.11×
16 → 32     2.14×
32 → 64     1.42×   ← workers already past GIL saturation; further additions wait
```

**This is the GIL signature.** Adding workers within a single CLCR pod
doesn’t add throughput — it just queues more concurrent scrape parses onto
the pod’s single CPython interpreter. The 32→64 step ratio dropping below 2×
is *not* a “less contention” win; it’s that 32 workers were already enough to
saturate the GIL and the additional 32 just sit on the runqueue waiting for
their interpreter slice.

**The runs/min/check column proves it.** Across N=1 to N=64 (a 64×
nominal-capacity range), runs/min/check stays in a tight 0.81–1.18 band.
**Total throughput is essentially flat.** All those extra workers buy you
*literally zero* additional check completions per minute.

## Where the customer’s `WorkersNeeded=1.0` cap comes from

The dispatcher computes `WorkersNeeded = min(1.0, avg_exec_time / interval)`
(handoff §7, dispatcher_rebalance.go:298–302). With `min_collection_interval`
default 15s:

```
N  exec_mean   WorkersNeeded   notes
1  0.535s      0.036           tiny load
2  1.305s      0.087
4  2.310s      0.154
8  4.690s      0.313           still well below cap
16 9.880s      0.659           starting to look heavy
32 21.159s     1.000 (capped)  ← exec >= interval
64 30.064s     1.000 (capped)  ← exec 2× interval
```

The customer’s `agent clusterchecks` snapshot at `DD_CHECK_RUNNERS=64` shows
“many checks at exactly 1.0” — consistent with our N=32 and N=64 cells where
`exec_mean` exceeds the 15s interval and `WorkersNeeded` clamps at the cap.

Once every check pegs at WorkersNeeded=1.0, the rebalancer can no longer
distinguish checks by load. Moving the orchestrator check (its true
`WorkersNeeded` is ~0.04) shaves real utilization from the source runner; the
rebalancer takes the bait every 10 minutes (the reproduction we captured in
`runs/rebalance-reproduction-*`).

## Answer to “is there a knee”

**No useful knee in N.** Throughput is GIL-limited and constant. The only
thing that changes with N is wall-clock exec time per check (linearly worse)
and what fraction of checks the rebalancer sees as pegged at the
WorkersNeeded cap (eventually all of them).

The real lever is **CLCR replica count**, not workers per pod: each replica
is a separate Python interpreter with its own GIL. The customer’s eventual
stable config (`DD_CHECK_RUNNERS=16`, 9 replicas — internal SME consultation) is
9 GILs running in parallel, not 1 GIL with 144 workers. With 9 GILs each
running ~16 effective-parallel scrapes, the customer gets ~9× the throughput
they’d have at `DD_CHECK_RUNNERS=144` × 1 replica.

## Practical guidance

For a Python-openmetrics-heavy workload:

- Pick `DD_CHECK_RUNNERS` near where p95 exec time approaches your scrape
  interval (in our setup that’s **N ≈ 8–16** for `interval=15s`).
- Scale **replicas** to absorb load past that. Each replica is one more GIL.
- Don’t set N high under the assumption it’ll “use more CPU per pod” — past
  the GIL knee, additional workers add wall-clock latency but no throughput.
- Watch `agent clusterchecks` for `WorkersNeeded` saturation. If most checks
  are at 1.0, you’re past the knee.
