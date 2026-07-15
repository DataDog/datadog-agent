# Documented parser strictness differences

## Status

Informational. These are low-risk compatibility differences on malformed or
ambiguous payloads. No implementation change is proposed without a concrete
customer or exporter compatibility requirement.

## Known differences

### UTF-8 label values

- Go decodes label values as UTF-8.
- Python may preserve raw bytes from older parser behavior.
- Go matches the current OpenMetrics UTF-8 requirement.

### Reserved `__` label prefix

- Python rejects labels using the reserved `__` prefix.
- Go may accept them.
- Exporters should not emit reserved labels.

### Non-numeric summary quantiles

For a summary sample such as:

```text
request_latency{quantile="median"} 1
```

- Python rejects the non-numeric quantile.
- Go may treat the quantile label as opaque text.
- The payload is invalid under normal summary semantics.

### Raw newlines in label values

- Python may lose parser synchronization and reject later samples.
- Go's line-oriented handling may fail or recover differently.
- Raw newlines must be escaped in valid exposition text.

## Removed from this list

Duplicate label names are no longer a known difference: current Go parsing
explicitly rejects duplicate labels, matching Python's rejection behavior.

## Impact

Real-world risk is low because compliant exporters do not emit these forms.
The differences matter when migration compatibility with a specific malformed
third-party endpoint is more important than strict format enforcement.

## Decision rule

Promote an item to an actionable bug only when at least one condition holds:

- A supported exporter emits the payload.
- A customer relies on Python's behavior.
- The specification clearly requires one behavior.
- The difference can suppress otherwise valid metrics from the same endpoint.

## Verification

Keep one focused test per documented difference so behavior changes are
intentional. Each test should state whether Go accepts, rejects, or partially
recovers the payload; it should not silently suppress a newly resolved case.
