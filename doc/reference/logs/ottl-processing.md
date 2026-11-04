# OTTL processing rules for JSON logs

The Logs Agent supports OTTL (OpenTelemetry Transformation Language) conditions
for filtering JSON log messages. Use field names and nested paths to select the
logs to send to Datadog.

## Integration configuration

Configure rules in the log source's `log_processing_rules` list, for example in
`conf.d/my_app.d/conf.yaml`:

```yaml
logs:
  - type: file
    path: /var/log/my_app.json
    service: my_app
    source: my_app
    log_processing_rules:
      - type: include_at_match_ottl
        name: keep_production
        pattern: 'attributes["env"] == "production"'
      - type: exclude_at_match_ottl
        name: drop_health_checks
        pattern: 'attributes["http"]["path"] == "/health" and attributes["http"]["status"] < 400'
```

The rules above retain production logs and remove successful health checks.
The JSON fields are read from the message content, rather than Agent tags or
source metadata.

## Global configuration

Use `logs_config.processing_rules` in `datadog.yaml` to apply conditions to every
log source:

```yaml
logs_enabled: true
logs_config:
  processing_rules:
    - type: exclude_at_match_ottl
      name: drop_debug
      pattern: 'attributes["level"] == "debug"'
```

Global processing rules run before source processing rules. Each list executes
in its configured order and may mix OTTL conditions with existing processing
rule types.

## Conditions and errors

| Rule type | Behavior |
| --- | --- |
| `exclude_at_match_ottl` | Drop the message when the condition is true. |
| `include_at_match_ottl` | Drop the message when the condition is false. |

Every rule requires a name and a nonempty, valid OTTL condition in `pattern`.
Invalid conditions are rejected during configuration validation.

An evaluation error passes the message through that rule; later rules still
execute. This includes malformed JSON, unsupported value conversions, and
out-of-range array indexes. Missing fields evaluate to `nil`, allowing conditions
such as `attributes["env"] != nil`. Because errors pass through, include rules do
not guarantee exclusion of malformed or non-JSON messages.

## Available data

- `attributes["field"]` reads a field from the message's JSON object.
- `attributes["http"]["status"]` reads a nested field.
- `attributes["events"][0]["name"]` reads an array element.
- `body` reads the complete message text, including for non-JSON messages.
- `log.attributes` and `log.body` are aliases for these paths.

String keys, integer array indexes, and computed keys are supported. Object
fields use bracket notation. JSON integers are retained as `int64`, including IDs
larger than 2^53; accessing an integer outside that range produces an evaluation
error. Duplicate object keys use the last value.

Conditions support upstream OTTL comparisons and boolean operators. The
registered functions are `IsMatch`, `HasPrefix`, `HasSuffix`, `IsString`, `IsInt`,
`IsDouble`, `IsBool`, `IsMap`, `IsList`, `Len`, `Int`, `Double`, and `String`.
Other converters and editor functions are not registered.

## Processing behavior

OTTL conditions are read-only. Kept messages retain their original JSON bytes.
Existing `mask_sequences` rules may change a message before a subsequent OTTL
condition reads it. In that case the subsequent condition sees the updated
content.

The Agent parses JSON lazily and shares the parsed message between OTTL rules.
The [benchmark notes](../../how-to/test/log-processing-ottl.md) describe this
implementation and its current performance measurements.
