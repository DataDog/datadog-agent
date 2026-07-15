# Out-of-range sample values abort the scrape instead of becoming infinity

## Summary

When a sample value exceeds the `float64` range, Go returns a parse error and
aborts the scrape. Python converts the value to positive or negative infinity
and continues.

## Reproduction

```text
# TYPE memory_growth gauge
memory_growth 1e400
```

The same issue applies to `-1e400`.

## Expected behavior

Submit `memory_growth` with `+Inf` or `-Inf`, matching Python and Go's own
`strconv.ParseFloat` result value.

## Actual behavior

Go returns:

```text
strconv.ParseFloat: parsing "1e400": value out of range
```

Python submits one gauge with value `+Inf`.

## Root cause

`strconv.ParseFloat` returns both an infinity value and `strconv.ErrRange` for
an overflow. The scraper treats every non-nil error as fatal and discards the
usable infinity value.

## Impact

One runaway counter, gauge, sum, or bucket value can suppress every metric from
the endpoint. Python continues collecting the unaffected metrics.

## Suggested fix

Accept the returned value when the error is `strconv.ErrRange`:

```go
value, err := strconv.ParseFloat(raw, 64)
if err != nil && !errors.Is(err, strconv.ErrRange) {
    return err
}
```

Apply the same policy everywhere sample numeric values are parsed. Timestamp
overflow should remain invalid because timestamps are converted to bounded
integer milliseconds.

## Verification

Assert parity for:

- `1e400` → `+Inf`
- `-1e400` → `-Inf`
- Gauge, counter, histogram sum/count/bucket, and summary values
- A valid metric following the overflow sample
- Invalid numeric syntax such as `1efoo` still follows the normal error or
  line-recovery policy
