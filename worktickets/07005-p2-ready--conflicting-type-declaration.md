# Conflicting `TYPE` declarations produce different metric kinds

## Summary

When a payload declares the same metric family with two different `TYPE`
values, Go uses the first declaration and Python uses the last. The migration
changes the submitted metric name and kind without reporting an error.

## Reproduction

```text
# TYPE requests gauge
requests 1
# TYPE requests counter
requests 2
```

## Expected behavior

Go should match Python's last-declaration-wins behavior for compatibility:

```text
kind: monotonic_count
name: <namespace>.requests.count
```

## Actual behavior

Go submits the family as a gauge using the first declaration:

```text
kind: gauge
name: <namespace>.requests
```

Python submits it as a monotonic count using the later counter declaration.

## Root cause

Go processes or caches the family type before the later declaration can replace
it. Python's metadata map is updated by each declaration before samples are
transformed.

## Impact

The metric still arrives, but its name and aggregation semantics change during
migration. Dashboards, monitors, and rate calculations can silently use the
wrong series.

Conflicting declarations are malformed exporter output, but Python accepts them
today, so compatibility requires a deterministic matching policy.

## Suggested fix

Use last-declaration-wins metadata for a family before transforming its samples.
If streaming constraints make that impossible, explicitly reject conflicting
declarations on both migration paths rather than silently producing a different
metric kind.

## Verification

Assert that:

- Gauge followed by counter matches Python's counter output.
- Counter followed by gauge matches Python's gauge output.
- Repeated identical declarations do not change output.
- Conflicts separated by unrelated families follow the same policy.
- The chosen policy is identical in streaming and buffered parser paths.
