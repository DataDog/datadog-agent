## Summary

Catalog of behaviors where the Go and Python OpenMetrics scrapers differ
because each is making a defensible-but-different interpretation of the
spec. None of these are clearly bugs in Go; they are documented here so
operators see this list (rather than discover them one at a time) and
so we have a single place to revisit if the desired Go behavior changes.

Filing as P3 / informational. No action required to ship the Go scraper.

## Context

During differential testing, the harness found five behaviors where Go
and Python disagree but neither implementation is unambiguously wrong:

1. **UTF-8 in label values** — Go decodes per modern OpenMetrics; Python
   preserves raw bytes (yielding mojibake when serialized as JSON).
   Go is technically more correct under the current spec; Python predates
   the UTF-8 mandate.

2. **`__`-prefixed label names** — Python rejects the entire sample
   (`prometheus_client` enforces the reserved namespace); Go accepts.
   Both are defensible. Most real-world exporters don't emit such
   labels.

3. **Duplicate label name within a sample** (`{foo="a",foo="b"}`) —
   Python rejects, Go accepts (last-value-wins). Both defensible.

4. **Non-numeric quantile** (`{quantile="median"}` on a summary) —
   Python rejects with `ValueError`, Go accepts the label as opaque.
   Both defensible.

5. **Raw newline inside a label value** (no `\n` escape) — Python's
   parser desyncs and rejects subsequent samples; Go accepts. Both
   technically wrong per spec, but observably divergent.

## Tracking

Each divergence has an adversarial case under
`pkg/collector/corechecks/openmetrics/differential/`:

| Divergence | Adversarial case |
|---|---|
| UTF-8 mojibake | (mutation-driven, no fixed case) |
| `__` reserved prefix | (mutation-driven via `opInjectReservedLabel`) |
| Duplicate label name | `TestOpenMetricsAdversarial/labels/duplicate_name` |
| Non-numeric quantile | `TestOpenMetricsAdversarial/summary/quantile_non_numeric` |
| Raw newline in value | `TestOpenMetricsAdversarial/labels/raw_newline_in_value` |

Suppression for these classes lives in
`differential/known_divergences.go` so they don't flap fuzz runs.

## Why this isn't P0/P1/P2

For each divergence:

- The metric in question is malformed by spec.
- Both Python and Go produce *some* sensible output (one of them
  emits the sample, one of them rejects it; nobody emits silently
  wrong data).
- Real-world incidence is low: most exporters don't emit malformed
  payloads, and when they do the operator typically wants to know
  via either error logs or missing metrics.

## When to revisit

Consider promoting any of these to P2 if:

- A customer reports unexpected behavior on a real exporter we don't
  control (e.g. a third-party tool that emits raw newlines).
- The spec gets clarified in a way that picks one behavior over the
  other.
- A post-migration audit reveals operators are confused by the
  inconsistency.

## Not in this ticket

The architectural per-line-recovery work (P0 ticket) is separate. That
one is required for production rollout; this catalog is purely for
reference.
