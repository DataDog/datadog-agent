# Memory diagnostics

These seven `heap_*` cases run automatically with the normal SMP suite. They retain the original 300 MiB/s workload, four CPUs, 2 GiB, V3 series and V2 sketches.

| Case | Purpose |
|---|---|
| `heap_control` | Unfiltered dual shipping |
| `heap_endpoint_1000`, `heap_endpoint_100000` | Primary endpoint allowlist with unfiltered secondary |
| `heap_mrf_1000`, `heap_mrf_100000` | Same allowed names and destinations using the existing MRF filter |
| `heap_mrf_unfiltered` | MRF-specific baseline without an allowlist |
| `heap_shared_100000` | Large policies on both endpoints sharing one encoder |

The MRF cases make port 9092 the ordinary primary and port 9091 the filtered failover destination, preserving the original destination policies. MRF rebuilds its lookup map per flush, whereas endpoint policies compile once. It is a control for the existing encoder architecture, not identical matcher costs. The shared case declares the policy twice, so its configuration storage is not identical to a single-policy case.

The collector records runtime memory counters, Agent and collector RSS/PSS separately, retry queue telemetry, and SHA-256-checked heap snapshots in target stdout. It does not force GC. A 37-second profile interval samples different positions in the 15-second flush cycle. The collector is present in all seven cases and adds overhead. Use the actual per-replicate measurement window, excluding warmup and SMP's separate profiler replicas.

Compare cases on the **comparison build**. Baseline-vs-comparison CI percentages do not isolate this feature. Check received throughput, drops, active contexts, successful forwarding, and retries before interpreting memory differences.

Heap profiles show allocation sites, not retaining owners. Use V3 dictionary and payload allocation stacks together with flush timing and queue telemetry. Retry telemetry does not cover every active/in-flight queue. Keep live objects, unreclaimed allocations, unused heap spans, and heap retained by the runtime distinct.

Regenerate or check from the repository root:

```sh
python3 test/regression/metrics-endpoint-routing/memory-diagnostics/generate.py
python3 test/regression/metrics-endpoint-routing/memory-diagnostics/generate.py --check
```

Recover full target stdout logs from SMP artifacts, rather than sampled log-search results. The extractor rejects incomplete profiles and emits pprof files, live-heap allocation summaries, and runtime counters:

```sh
python3 test/regression/metrics-endpoint-routing/memory-diagnostics/extract.py target.jsonl --output extracted
```
