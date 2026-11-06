# OTTL processing rules for logs

The Logs Agent supports OTTL (OpenTelemetry Transformation Language) conditions
for filtering JSON logs and statements for masking sensitive fields or message
text.

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
      - type: mask_ottl
        name: redact_password
        pattern: 'set(attributes["credentials"]["password"], "[REDACTED]") where IsMap(attributes["credentials"])'
```

These rules retain production logs, remove successful health checks, and redact
password fields. JSON fields come from the message content. Agent tags and
source metadata are separate from these attributes.

## Global configuration

Use `logs_config.processing_rules` in `datadog.yaml` to apply rules to every
log source:

```yaml
logs_enabled: true
logs_config:
  processing_rules:
    - type: exclude_at_match_ottl
      name: drop_debug
      pattern: 'attributes["level"] == "debug"'
```

Global rules run before source rules. Each list executes in its configured
order and may mix OTTL with existing processing rule types.

## Filtering

| Rule type | Behavior |
| --- | --- |
| `exclude_at_match_ottl` | Drop the message when the condition is true. |
| `include_at_match_ottl` | Drop the message when the condition is false. |

Filtering rules require a name and a nonempty, valid OTTL condition in `pattern`.
Invalid conditions are rejected during configuration validation.

For filtering, an evaluation error passes the message through that rule; later
rules still execute. Errors include malformed JSON, unsupported conversions, and
out-of-range array indexes. Missing fields evaluate to `nil`, allowing conditions
such as `attributes["env"] != nil`. Include rules therefore do not guarantee
exclusion of malformed or non-JSON messages.

## Available data

- `attributes["field"]` reads a field from the message's JSON object.
- `attributes["http"]["status"]` reads a nested field.
- `attributes["events"][0]["name"]` reads an array element.
- `body` reads the complete message text, including non-JSON messages.
- `log.attributes` and `log.body` are aliases for these paths.

String keys, integer array indexes, and computed keys are supported. Object
fields use bracket notation. JSON integers retain `int64` precision, including
IDs larger than 2^53. Reading an integer outside that range produces an
evaluation error; masking a different field preserves that integer's original
token. Duplicate keys use the last value.

The registered converters are `IsMatch`, `HasPrefix`, `HasSuffix`, `IsString`,
`IsInt`, `IsDouble`, `IsBool`, `IsMap`, `IsList`, `Len`, `Int`, `Double`, and
`String`. Masking adds the three editors below. Other functions are unregistered.

## Masking

Use `mask_ottl` with an OTTL statement in `pattern`. The replacement is part of
the statement; `replace_placeholder` is not used.

```yaml
log_processing_rules:
  - type: mask_ottl
    name: redact_password
    pattern: 'set(attributes["credentials"]["password"], "[REDACTED]") where IsMap(attributes["credentials"])'
  - type: mask_ottl
    name: redact_message
    pattern: 'replace_pattern(attributes["message"], "password=[^ ]+", "password=***")'
  - type: mask_ottl
    name: redact_body
    pattern: 'replace_match(body, "password=*", "password=***")'
```

The supported masking editors are:

- `replace_pattern(target, regex, replacement)` replaces matching string sections.
- `replace_match(target, glob, replacement)` replaces a whole matching string.
- `set(target, value)` assigns a string, integer, finite double, boolean, or JSON
  null. Body assignments require a string.

Statements support optional `where` conditions using the same paths and
converters as filters. Fields inside existing objects and array elements can be
masked; `set` can also add a final object field. Parent objects and array indexes
must already exist. Assigning objects or arrays is unsupported.

**Masking errors drop the message.** This follows the original `mask_ottl` rule
and differs from filtering's pass-through behavior. Errors include malformed
JSON when a JSON field is accessed, invalid parent types or array indexes, and
unsupported assignment types. Missing/non-string targets for `replace_pattern`
or `replace_match` are no-ops, following upstream OTTL behavior. A false `where`
condition leaves the message unchanged. Body-only statements can mask plain text
without parsing it as JSON.

Executed JSON masking statements normalize duplicate object keys to their last
value, including when the selected field needs no replacement. Shadowed copies
are omitted from output. Untouched integer tokens retain their precision,
including values larger than the OTTL `int64` range.

## Processing behavior

Filtering conditions are read-only. Later conditions and masking statements see
earlier edits. Existing `mask_sequences` rules can be mixed with OTTL rules in
the configured order.

JSON parsing is lazy and shared across rules. Adjacent JSON field masks update
the shared context and serialize once when a body/legacy rule or final output
needs bytes. Body edits invalidate the JSON cache. Output bytes belong to the
message and remain valid when the context returns to its pool.

Messages retain their original bytes when no edit or duplicate-key normalization
is needed. Changed JSON may be compacted and have normalized escaping. The
[benchmark notes](../../how-to/test/log-processing-ottl.md) describe the
implementation and performance measurements.

### Validation and compilation

Shared configuration parsing checks names, rule types, required fields, and regular expressions. The logs runtime compiles OTTL separately: global rules are checked when the logs agent or synchronous sender starts, and source rules are checked before the source is delivered to a launcher. `agent analyze-logs` uses the same full validation. Invalid global OTTL prevents logs startup; an invalid source remains visible with its rule name and parser error in Agent status and is not tailed. A structural config check alone does not validate OTTL syntax.

Compilation happens during setup, once for each rule definition. Compiled rules are shared by processing pipelines and inherited container sources. Global and source rules still share a pooled context and a lazy JSON parse per message. The shared config package only knows a small execution interface; it does not import OTTL, its parser, or its supported functions. This keeps those dependencies out of binaries that only consume logs configuration.
