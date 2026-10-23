# Endpoint-routing SMP experiments

This suite ports the matched allowlist-size sweep from [Saluki PR #2653](https://github.com/DataDog/saluki/pull/2653),
commit `a6027879262175933f5c358f1b5f33bf2cca35cb`. It measures the **Core Agent process**, including
DogStatsD ingestion, aggregation, serialization, compression and forwarding. ADP is explicitly disabled.
The routing implementation is included in this branch. See [source.json](source.json) for provenance.

## Run in CI

After reviewing, committing and pushing this branch, open the temporary benchmark PR against
`main`. A PR targeting another feature branch uses a light pipeline that skips SMP.
The existing **`single-machine-performance-regression_detector`** job runs these 14 cases alongside
the normal regression suite after the Agent image build and baseline check succeed.

Generated cases live in `test/regression/cases/matched_*`. They use the shared
`test/regression/config.yaml`, including Lading 0.33.0 and one native and one internal profiling
replica. Ordinary replica count, warmup, capture duration and timeout use the standard suite
settings. There are no dedicated submission overrides or GitLab changes for this benchmark.
Check the actual run settings when comparing with the ADP results, especially the measurement
window. Use unprofiled replicas for CPU/RSS comparisons.

**SMP's `baseline` and `comparison` labels are builds, not filtering off/on.** All cases use the same
configuration for both builds. For feature overhead, compare each `matched_exact_*` or
`matched_prefix_*` with `matched_control` in the **same candidate build and run**, then inspect the
per-replicate CPU/RSS spread. The baseline build may not implement the endpoint policy at all.
The ordinary branch-vs-main report answers a different question and includes any other build changes.

## Cases and unchanged workload

| Scenarios | List entries | Optimization goals |
|---|---:|---|
| `matched_control` | none | CPU, memory |
| `matched_exact_1000`, `matched_exact_10000`, `matched_exact_100000` | 1K / 10K / 100K exact | CPU, memory |
| `matched_prefix_1000`, `matched_prefix_10000`, `matched_prefix_100000` | 1K / 10K / 100K literal prefixes | CPU, memory |

Only the primary (`http://127.0.0.1:9091`) is filtered. The secondary (`http://127.0.0.1:9092`)
remains unfiltered. All generated workload names are admitted at both destinations. Matching is
not intended to reduce output in this sweep: it exposes the cost of a separate filtered encoding
path at equal workload output.

The source workload is retained:

- 100 base names, `routing.metric.10000` through `routing.metric.10099`.
- Approximately 100,000 contexts, sampled by Lading from randomized tags.
- 300 MiB/s offered over a Unix datagram socket; fixed 512 MiB prebuilt cache and the original seed.
- Count/gauge/distribution/set/histogram weights 208/66/72/9/1; timestamped input disabled.
- 2–50 tags, 3–150-character tags and the same multivalue packing settings.
- Four CPUs and 2 GiB target memory for every case.
- Two HTTP blackholes; V3 series explicitly enabled for both and V2 sketches, using zstd.
- Histogram outputs pinned to `.max`, `.median`, `.avg`, `.count` and `.95percentile`;
  histogram-to-distribution copying disabled.
- Lading 0.33.0, matching the source experiment and the existing Agent SMP suite.

The numbers 1K/10K/100K are **configured list lengths**, not active names or contexts. Extra entries
are absent filler names/prefixes. Prefixes have fixed-width numeric suffixes, so the matcher cannot
collapse the requested policy size. This is not a worst-case test of 100K simultaneously active names.

## Necessary Agent adaptations

- Run `/opt/datadog-agent/bin/agent/agent run` with `datadog.yaml`, rather than the ADP executable
  and ADP configuration path. `data_plane.enabled: false` also opts out of the ADP preflight process.
- Use `/tmp/dsd.socket` for both receiver and Lading and scrape Core Agent Prometheus telemetry at
  `http://127.0.0.1:5000/telemetry`, tagged `sub_agent: core`; ADP's port 5100 is not used.
- Do not copy the ADP-only context-limit or string-interner settings. The Agent has different
  aggregation/storage internals; observed context count and drops must be checked in CI.
- An empty `conf.d` prevents integration check workloads. Cloud metadata, metadata collection,
  Agent telemetry submission, logs, APM and remote configuration are disabled; Prometheus telemetry
  remains enabled in every case. This isolates the Core Agent's metrics path, not the container's
  complete default product workload.
- The Agent emits background series even with no checks. Exact policies reserve four entries for
  `datadog.agent.started`, `datadog.agent.running`, `datadog.agent.python.version` and
  `datadog.agent.check_ready`, replacing four absent fillers. They contain 600 matching workload
  names, four background names, and `size - 604` fillers. Prefix policies include `datadog.` plus
  the 100 matching workload prefixes and `size - 101` fillers. Both still have exactly the requested
  list size. This avoids dropping known Agent background metrics only in the filtered cases.
  Recheck this set if the Agent begins emitting additional background names.
- Use the existing Agent profiling setup (one native and one Go internal profiling replica),
  instead of ADP's two native profiling replicas. Performance comparisons use unprofiled replicas.

## Configuration review and interpretation gates

The source configuration has no obvious static error in the matched-name/list construction.
A literal transplant would run the wrong executable, scrape the wrong telemetry endpoint and
omit Agent background metrics; the adaptations above correct those differences.

Before treating the measurements as equal-input/equal-output overhead:

1. Confirm every case sustains comparable received bytes/s and processed metrics/s. Inspect the
   DogStatsD UDS/processed metrics, parsing and packet-drop counters, active contexts and flush
   backlog. Offered 300 MiB/s alone is not proof the Agent processed 300 MiB/s.
2. Check successful forwarding to **both** blackholes and inspect errors, retries, points and bytes.
   Payload packing/compression and flush timing can vary; equal wire byte counts are not a substitute
   for equal decoded metric coverage. These SMP blackholes do not assert received metric names.
3. Check the observed name/context coverage: Lading samples names and the context range is
   100000–100001, so configuration alone does not prove every name or exactly 100000 contexts was seen.
4. If any case is saturated, dropping metrics, OOMing or backing up, rerun **all seven scenarios**
   at the same lower offered rate before inferring feature overhead. Change only
   `templates/lading.yaml`, regenerate and keep the new rate consistent across both CPU/memory cases.
5. Compare percentage changes within each implementation. Core Agent and standalone ADP have
   different process baselines, aggregation behavior and telemetry; raw cross-product RSS or CPU
   differences do not isolate feature cost.

The CI performance run is still required. Local configuration checks do not establish target
capacity or validate blackhole contents. No CI job has been submitted from this task.

## Generate and check

Requires Python 3 and PyYAML (available in the repository's development Python environment):

```sh
python3 test/regression/metrics-endpoint-routing/generate.py
python3 test/regression/metrics-endpoint-routing/generate.py --check
```

Edit `templates/` or `generate.py`. Do not hand-edit the generated `../cases/matched_*` files. The generator checks
list cardinality, deduplication/prefix compaction, endpoint identities, socket agreement, histogram
outputs, protocol settings and background coverage. `--check` detects stale, missing or unexpected
files within these generated cases. Existing regression cases are left untouched. Commit the generated files: the SMP submission reads them directly, with no Saluki checkout
or generator execution required in CI. Large policy YAML stays in files, avoiding environment-variable
size limits at 100K entries.
