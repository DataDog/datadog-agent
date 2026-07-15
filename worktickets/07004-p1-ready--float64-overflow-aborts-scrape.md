## Current verification after Ali branch update

Still open. The now-runnable focused differential case reproduces unchanged:
Go returns zero submissions with `strconv.ParseFloat ... value out of range`;
Python submits one `+Inf` gauge.

---

## Summary

The Go scraper aborts the entire scrape with
`scrape: strconv.ParseFloat: parsing "<value>": value out of range` when
a sample value exceeds float64 range (e.g. `1e400`). Python clamps the
value to `±Inf` and submits it like any other.

A single runaway metric (counter wraparound, memory leak observed in a
gauge, exporter bug) zeros out the Go-side scrape. Python keeps reporting
the other metrics and the runaway one as `+Inf`, which is the right
graceful-degradation behavior.

## Context

`strconv.ParseFloat` returns `(±Inf, ErrRange)` for values outside
float64's representable range — `Inf` is a valid float64 in Go. The
Go scraper currently treats `ErrRange` as fatal; Python's float
constructor accepts the same input and just produces `inf`.

The minimal repro:

```
# TYPE m gauge
m 1e400
```

## Repro

Adversarial catalog:

```bash
go test -tags openmetrics_differential -v \
    -run 'TestOpenMetricsAdversarial/values/over_max_float64' \
    ./pkg/collector/corechecks/openmetrics/differential/
```

Current output:

```
go_err: scrape: strconv.ParseFloat: parsing "1e400": value out of range
    while parsing: "m 1e400"
```

Python returns 1 gauge submission with value `+Inf`.

## Suggested fix

Where `strconv.ParseFloat` is called on a sample value, treat
`ErrRange` as non-fatal and use the returned `±Inf` value:

```go
v, err := strconv.ParseFloat(s, 64)
if err != nil {
    var nerr *strconv.NumError
    if errors.As(err, &nerr) && errors.Is(nerr.Err, strconv.ErrRange) {
        // v is already ±Inf per strconv contract; use it.
    } else {
        return err  // syntax error, real failure
    }
}
```

Same logic for histogram bucket counts, summary quantile values,
`_count`, and `_sum` — anywhere a numeric value is parsed.

## Severity rationale

P1 because:
- Runaway metrics happen in the wild (memory leak gauge climbs past
  1e300; counter wraps; misconfigured rate calculation overflows).
- Failure is total — losing every metric from an affected endpoint.
- Fix is a small typed-error check, no behavior-shape change.

## Verification

After fix:

```
TestOpenMetricsAdversarial/values/over_max_float64   # passes with Go emitting +Inf
```

And the `float64_overflow` entry in `known_divergences.go` can
be removed.

## Out of scope

Negative-overflow (`-1e400` → `-Inf`) and subnormal handling are
adjacent but not the same bug. Subnormals (`5e-324`) already work —
see `TestOpenMetricsAdversarial/values/smallest_subnormal` which
passes today.
