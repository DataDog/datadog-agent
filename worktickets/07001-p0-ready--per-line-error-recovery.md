## Summary

The Go OpenMetrics scraper currently aborts the entire scrape on *any* parse
error. The Python implementation it's replacing skips the offending line and
continues. This single behavioral difference is the root cause of three
high-severity bugs filed separately in this folder, and of essentially every
divergence the fuzz/mutation harness has discovered.

**This must be resolved before the Go scraper replaces the Python one in
production.** A misbehaving exporter that emits one malformed line will zero
out a Go-side scrape that Python would have handled gracefully — losing all
the metrics from that endpoint until the operator notices.

## Context

This branch (`sopell/openmetrics-core-check-go`) adds the Go OpenMetrics
scraper from scratch — there is no upstream to file against. These tickets
document divergences against the production Python check
(`datadog_checks.base.checks.openmetrics.v2.OpenMetricsBaseCheckV2`) that
need to be addressed before/during the rollout, captured here while the
differential harness still has fresh repros.

## Repro

The full repro space is captured in
`pkg/collector/corechecks/openmetrics/differential/`. Two narrow examples
from the catalog:

```bash
go test -tags openmetrics_differential -v \
    -run 'TestOpenMetricsAdversarial/format/openmetrics_exemplar' \
    ./pkg/collector/corechecks/openmetrics/differential/
```

```bash
go test -tags openmetrics_differential -v \
    -run 'TestOpenMetricsAdversarial/values/over_max_float64' \
    ./pkg/collector/corechecks/openmetrics/differential/
```

In both, Go returns zero submissions plus an error from `scraper.Scrape()`;
Python returns N submissions and logs/swallows the bad line.

## Evidence from the weekend fuzz run

8 hours of coverage-guided fuzzing produced 1202 distinct failing inputs.
After triage, every single one reduces to one of three Go error messages:

```
scrape: expected a valid start token, got <byte>
scrape: expected equal, got <byte>
scrape: strconv.ParseFloat: parsing <token>: invalid syntax
```

…and the Python check tolerated all 1202 inputs (returned partial
submissions). The fuzz engine has effectively mapped the entire "Go
aborts on byte-level parse error" surface; it isn't discovering anything
new because the bug is structural, not in any particular code path.

## Suggested fix shape

Replace the early-return-on-error pattern in the scrape loop with a
per-line `try { parse_and_emit } catch { log_and_continue }` pattern,
matching the Python check's
`base_scraper.py:284 consume_metrics` semantics:

- Errors at the family-header level (`# HELP`, `# TYPE`, `# UNIT`,
  `# EOF`) should log and skip the family, then resume at the next
  metric family.
- Errors mid-sample-line should log and skip the line, then resume at
  the next line.
- Errors that prevent ANY parse progress (e.g. bad gzip framing, empty
  body) can still error the scrape — those are HTTP-level problems,
  not payload-level.

Maintain a per-scrape counter of skipped lines and emit it as a service
check or telemetry metric (e.g. `openmetrics.lines_skipped`) so
operators can tell when their exporter is misbehaving.

## Severity rationale

This is P0 because shipping the Go scraper as a drop-in replacement
*without* this fix is a regression: anything that worked under Python
today and emits even one malformed line stops working entirely under
Go. Quiet partial degradation is the right behavior for an observability
tool.

## Related tickets (symptoms of this bug)

- TYPE-keyword rejection (OpenMetrics-only `stateset`, `gaugehistogram`, `info`)
- Exemplar trailer rejection (OpenMetrics 1.0.0 feature)
- Float64 overflow rejection

These three are filed separately at P1 because each has a narrow,
local fix (e.g. accept-and-coerce instead of error) that's cheaper
than the full per-line recovery refactor. They can be fixed independently
or all subsumed by the per-line recovery work.

## Verification

The fix should turn the following currently-failing subtests green:

```
TestOpenMetricsAdversarial/format/openmetrics_exemplar
TestOpenMetricsAdversarial/values/over_max_float64
TestOpenMetricsAdversarial/format/bare_type_keyword
TestOpenMetricsAdversarial/format/bare_help_keyword
TestOpenMetricsAdversarial/format/leading_form_feed
TestOpenMetricsAdversarial/name/numeric_metric_name
```

Plus most of the suppressed classes in `known_divergences.go` should
become removable (the `go_parse_error_*` family).
