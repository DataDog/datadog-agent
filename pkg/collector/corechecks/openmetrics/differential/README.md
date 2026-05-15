# OpenMetrics differential testing harness

Throwaway differential-testing tool that runs the Go OpenMetrics check
(`pkg/collector/corechecks/openmetrics`) and the upstream Python OpenMetrics
base check against byte-identical input payloads, then asserts they emit
equivalent submission sets.

This is **not** wired into normal CI. It's gated behind the
`openmetrics_differential` Go build tag so it stays invisible to `dda inv test`,
`go test ./...`, etc. It exists to give the Go port a one-shot, high-confidence
parity check against the production Python implementation on real fixtures
that the Go check has never seen.

## How to run it

Prereqs:

- `uv` on `PATH` (the Python sidecar is a [PEP 723 inline-metadata](https://peps.python.org/pep-0723/) script)
- a local clone of `integrations-core` at `~/dd/integrations-core` (the sidecar
  pulls `datadog-checks-base` from there via a `file://` dependency — edit
  `sidecar.py` if your clone lives elsewhere)

```bash
go test -tags openmetrics_differential -v \
    ./pkg/collector/corechecks/openmetrics/differential/...
```

The first run is slow (uv creates the virtualenv); subsequent runs are
fast (the sidecar process is long-lived and reused across subtests).

## How it works

```
         httptest.Server
        /               \
       /                 \
  Go scraper          Python sidecar (subprocess)
      |                    |
  RecordingSender      patched OpenMetricsBaseCheckV2
      |                    |
      \------ diff --------/
```

- A single `httptest.Server` per fixture serves the gzipped Prometheus payload.
  Both implementations dial the same URL, so neither can drift on parsing
  because of upstream-bytes differences.
- The Go side uses `openmetrics.NewScraperFromYAML(...)` and a `RecordingSender`
  that implements `sender.Sender` and appends every submission to a slice.
- The Python side is a long-lived subprocess speaking JSON-lines over
  stdin/stdout. It monkey-patches `OpenMetricsBaseCheckV2.gauge` /
  `.count` / etc. *on the class, before instantiation* (transformers grab
  bound methods at config time — patching after `__init__` is too late).
- `CompareSubmissions` builds a multiset keyed by `(kind, name, sorted tags,
  hostname)`, normalizes tag values that legitimately vary across runs
  (`endpoint:` carries the httptest server's random port), and applies a
  relative float tolerance to value comparison.

## Fixtures

Reuses `../testdata/upstream_benchmarks/`:

| Fixture        | Submissions | Notes                                        |
|----------------|-------------|----------------------------------------------|
| `ksm`          | 744         | mostly gauges, heavy on labels-as-tags       |
| `msk_jmx`      | 11          | sparse, exercises `_count`/`_total` handling |

Both currently use the `metrics: [".+"]` wildcard. Adding more fixtures is a
matter of dropping a gzipped Prometheus payload under
`../testdata/upstream_benchmarks/` and appending an entry to `fixtureCases`.

## Known normalizations (and what they hide)

`diff.go` strips/normalizes a handful of differences that are *not* check
behavior:

- `endpoint:<url>` tag values — `httptest.Server` picks a random port per run.
  Presence of the tag is still asserted because both sides should add it
  together; only the value differs.
- Service-check `message` text — wording diverges between runtimes by design.
- Float values via relative tolerance (`1e-9`). String round-trips through
  Prometheus text format are not exact.
- Python namespace prefix is reapplied in `sidecar.py` because we replace
  `gauge`/`service_check` directly and bypass `AgentCheck._format_namespace`.
  The Go side always emits namespaced names; the sidecar mirrors that
  (honoring `raw=True` opt-out).

If you suspect the harness is *hiding* a real bug, the right move is to
remove the normalization, re-run, and triage.

## Out of scope

This harness checks *corpus-level* parity on captured payloads. It does **not**
cover the five documented behavioral gaps from the Go port (dynamic
`prometheus_url`, `set_dynamic_tags`, etc.) — those are deliberate divergences
tracked elsewhere and exercised through different fixtures or are explicit
`errUnsupportedCoreConfig` fallbacks.

## Lifecycle

Delete this directory once the Go check has been in production for one full
release cycle without parity regressions reported. The build-tag gating means
there is no runtime cost to leaving it in tree, but it does add maintenance
weight (the Python sidecar depends on the upstream check's internal API).
