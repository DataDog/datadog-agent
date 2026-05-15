# OpenMetrics differential testing harness

Throwaway differential-testing tool that runs the Go OpenMetrics check
(`pkg/collector/corechecks/openmetrics`) and the upstream Python OpenMetrics
base check against byte-identical input payloads, then asserts they emit
equivalent submission sets.

This is **not** wired into normal CI. It's gated behind the
`openmetrics_differential` Go build tag so it stays invisible to `dda inv test`,
`go test ./...`, etc. It exists to give the Go port a high-confidence parity
check against the production Python implementation on real and synthetic
inputs.

Three entry points, increasing in coverage:

1. `TestOpenMetricsDifferential` — fixed corpus, two real captured payloads.
2. `TestOpenMetricsMutation` — N random mutations of each corpus payload.
3. `TestOpenMetricsAdversarial` — hand-crafted spec-corner cases, each
   targeting a specific behavior class so failures triage by subtest name.
4. `FuzzOpenMetricsDifferential` — `testing.F` fuzz target seeded by the
   corpus, coverage-guided once run under `-fuzz`.

All four share the same Python sidecar, payload server, and diff machinery.

## Prereqs

- `uv` on `PATH` (the Python sidecar is a [PEP 723 inline-metadata](https://peps.python.org/pep-0723/) script)
- a local clone of `integrations-core` at `~/dd/integrations-core` (the sidecar
  pulls `datadog-checks-base` from there via a `file://` dependency — edit
  `sidecar.py` if your clone lives elsewhere)

## How to run

### Corpus replay (fast, no surprises expected)

```bash
go test -tags openmetrics_differential -v -run TestOpenMetricsDifferential \
    ./pkg/collector/corechecks/openmetrics/differential/
```

### Mutation differential (a few seconds, often surfaces bugs)

```bash
go test -tags openmetrics_differential -v -run TestOpenMetricsMutation \
    ./pkg/collector/corechecks/openmetrics/differential/ \
    -mutation.iters=50 -mutation.ops=2 -mutation.seed=1
```

Flags:

| Flag                  | Default | Meaning                                                    |
|-----------------------|---------|------------------------------------------------------------|
| `-mutation.iters`     | 50      | mutated payloads per seed fixture                          |
| `-mutation.ops`       | 3       | mutations applied per iteration                            |
| `-mutation.seed`      | 0       | RNG seed (0 = derive from PID for some variability)        |
| `-mutation.failfast`  | false   | stop at first divergence instead of running the full budget |

On any divergence the mutated payload is dumped to
`testdata/regressions/<sha>.prom` with a `.meta` sidecar that captures the
verdict, error strings, and submission counts. That directory is gitignored —
these are session-local triage artifacts, not durable test fixtures.

### Adversarial (hand-crafted spec corners, ~half second)

```bash
go test -tags openmetrics_differential -v -run TestOpenMetricsAdversarial \
    ./pkg/collector/corechecks/openmetrics/differential/
```

Each case in `adversarial.go` becomes a named subtest — e.g.
`TestOpenMetricsAdversarial/format/openmetrics_exemplar`. Use `-run` to
focus, e.g. `-run 'TestOpenMetricsAdversarial/histogram'`.

Adding a case is cheap: append a literal to one of the `*Cases()` functions.
Categories currently covered: histogram corners (non-monotonic, missing
+Inf, NaN sum...), summary corners (out-of-range quantile, duplicate
quantile...), label corners (duplicate name, very long value, raw
newline...), metric-name conflicts (conflicting TYPE, `_total` collision),
format mixing (OpenMetrics exemplars/UNIT/EOF in Prometheus payloads),
value rendering (hex floats, float64 overflow, explicit `+` sign).

### Fuzz mode (long-running, coverage-guided)

```bash
# replay seed corpus only (fast)
go test -tags openmetrics_differential -v -run FuzzOpenMetricsDifferential \
    ./pkg/collector/corechecks/openmetrics/differential/

# run the fuzz engine for 5 minutes
go test -tags openmetrics_differential \
    -fuzz FuzzOpenMetricsDifferential -fuzztime 5m \
    ./pkg/collector/corechecks/openmetrics/differential/
```

Go's fuzz engine accumulates discovered inputs under
`testdata/fuzz/FuzzOpenMetricsDifferential/`. That directory is also
gitignored — promote any input you want to keep as a permanent regression
seed by moving it into a `f.Add(...)` call in `fuzz_test.go`.

## How it works

```
         single httptest.Server (atomic payload swap)
        /               \
       /                 \
  Go scraper          Python sidecar (long-lived subprocess)
      |                    |
  RecordingSender      patched OpenMetricsBaseCheckV2
      |                    |
      \------ diff --------/
```

Key design points:

- **One server, one sidecar, many iterations.** The payload server uses
  `atomic.Pointer[[]byte]` so per-iteration payload swap is contention-free.
  The sidecar process spans the whole test — first-run uv environment
  creation is amortized across thousands of iterations.
- **Python sidecar re-applies the namespace prefix** that we bypass by
  monkey-patching `OpenMetricsBaseCheckV2.gauge` directly (the patch skips
  `AgentCheck._format_namespace`). The patch honors `raw=True` for callers
  that opt out of namespacing.
- **Diff normalization** for legitimately varying things only: the random
  `endpoint:<url>` tag value (httptest port changes per process start) and
  service-check `message` text. Float values use a relative tolerance.
- **Verdict bucketing** treats four outcomes distinctly: `agree`,
  `both_rejected`, `divergent`, `go_rejected_py_accepted`,
  `go_accepted_py_rejected`. Only the last three fail the test —
  `both_rejected` is agreement under the harness contract.

## Mutator

`mutate.go` is a deterministic, seeded, line-oriented mutator. Mutations are
text-level edits to Prometheus payload bytes — no AST. Op weights are tuned
to favor mutations that surface high-signal divergences over those that
produce trivially-broken inputs.

Current ops:

| Op                      | Weight | Effect                                               |
|-------------------------|--------|------------------------------------------------------|
| `perturb_value`         | 6      | NaN, ±Inf, subnormal, 2^53+1, negate, mul ×1.0001    |
| `mutate_label`          | 4      | replace label value with empty/unicode/escape-heavy  |
| `duplicate_sample`      | 3      | duplicate a sample line                              |
| `drop_sample`           | 3      | drop a sample line                                   |
| `swap_samples`          | 2      | swap two sample lines                                |
| `inject_junk_label`     | 2      | append a non-reserved label                          |
| `corrupt_type`          | 2      | replace TYPE keyword (counter↔gauge↔stateset…)        |
| `non_monotonic_bucket`  | 2      | reverse histogram bucket order                       |
| `drop_help` / `drop_type` | 1+1  | remove HELP / TYPE meta line                         |
| `inject_blank`          | 1      | inject blank line                                    |
| `inject_comment`        | 1      | inject random `# ...` comment                        |
| `inject_reserved_label` | 1      | inject `__reserved_*` label (rare, well-known div.)  |
| `truncate_value`        | 1      | truncate sample mid-value                            |

Determinism contract: `NewMutator(seed).Mutate(input, n)` produces identical
output for identical (seed, input, n). When a fuzz finds a failure, the
`-mutation.seed=N` flag alone is enough to reproduce.

## Findings so far

Findings discovered by running the harness against captured payloads,
mutated payloads, and hand-crafted adversarial cases:

### Real Go bugs (production-relevant)

1. **OpenMetrics-only TYPE keywords (`stateset`, `gaugehistogram`, `info`)
   abort the entire Go scrape.** Python degrades gracefully and emits all
   other samples. Repro: change any `# TYPE foo gauge` line in `ksm.txt.gz`
   to `# TYPE foo stateset`. Likely the same code path as the
   Prometheus-vs-OpenMetrics text format distinction — the Go scraper
   appears to be strict-Prometheus only.
2. **OpenMetrics exemplar trailers abort the entire Go scrape.** Any
   sample line ending in `# {trace_id="..."} 0.0 1620000000.0` is
   rejected with `expected timestamp or new record, got "#"`. Python
   handles it. Modern OpenMetrics-compliant endpoints emit exemplars for
   trace correlation; this is a high-impact production bug. Adversarial
   case: `format/openmetrics_exemplar`.
3. **Float overflow (`1e400`) aborts the entire Go scrape** with
   `strconv.ParseFloat: value out of range`. Python clamps to `+Inf` and
   submits. A single runaway metric (counter wraparound, memory leak,
   exporter bug) takes the whole scrape down on the Go side. Adversarial
   case: `values/over_max_float64`.
4. **Conflicting TYPE for the same metric name diverges silently.** Go
   honors the first `# TYPE` declaration, Python honors the last. A buggy
   exporter that emits `# TYPE m gauge` then `# TYPE m counter` produces
   different submission types on the two impls (gauge vs monotonic_count
   with `.count` suffix). Adversarial case: `name/conflicting_type`.

### Spec-strictness divergences (defensible either way)

5. **UTF-8 label values: Go decodes, Python preserves raw bytes** (mojibake).
   Go is correct per modern OpenMetrics; `prometheus_client` predates the
   UTF-8 mandate. Not actionable for Go.
6. **`__`-prefixed label names**: Python rejects the entire sample
   (`Reserved label metric name`), Go accepts. Both behaviors defensible.
7. **Duplicate label name within a sample** (`{foo="a",foo="b"}`): Python
   rejects, Go accepts (last value wins). Both defensible.
8. **Non-numeric quantile** (`{quantile="median"}` on a summary): Python
   rejects, Go accepts. Both defensible.
9. **Raw newline inside a label value**: Python parser desyncs and
   rejects subsequent samples; Go accepts. Both implementations are
   technically wrong; Go is more permissive.


## Out of scope

This harness checks *single-scrape* parity. It does **not** cover:

- The five documented behavioral gaps from the Go port (dynamic
  `prometheus_url`, `set_dynamic_tags`, etc.) — those are deliberate
  divergences tracked elsewhere.
- Stateful multi-scrape behavior (rate calculation across two scrapes,
  counter cache eviction, etc.). Out of scope; too much harness complexity
  for the marginal signal.
- CI integration. Stays a build-tag-gated, locally-invoked workflow.

## Lifecycle

Delete this directory once the Go check has been in production for one full
release cycle without parity regressions reported. The build-tag gating means
there is no runtime cost to leaving it in tree, but it does add maintenance
weight (the Python sidecar depends on the upstream check's internal API and
the mutator depends on Prometheus text-format conventions).
