> **TL;DR:** Wraps the gojq library to run jq-style queries against JSON or YAML data, adding a compiled-query cache and a YAML normalization helper for agent compliance and autodiscovery use cases.

# pkg/util/jsonquery

## Purpose

`pkg/util/jsonquery` wraps the [`gojq`](https://github.com/itchyny/gojq) library (a pure-Go jq implementation) and adds two agent-specific conveniences: a compiled-query cache backed by `pkg/util/cache` (6-hour TTL), and a YAML normalization helper that converts the output of `go-yaml` into a form that gojq can process. It is the single point of entry for running jq-style queries against JSON or YAML data in the agent.

---

## Key elements

### `jsonquery.go`

**`Parse(q string) (*gojq.Code, error)`** — parses and compiles a jq expression, caching the result under the key `"jq-<q>"`. Subsequent calls with the same expression string return the cached `*gojq.Code` without re-parsing.

**`RunSingleOutput(q string, object interface{}) (string, bool, error)`** — runs a jq query against `object` and returns the first result as a string:

| Return | Meaning |
|---|---|
| `(value, true, nil)` | Query matched, `value` is the string representation of the result |
| `("", false, nil)` | Query produced no output or the result was `nil` |
| `("", false, err)` | Parse/compile/runtime error |

### `yaml.go`

**`NormalizeYAMLForGoJQ(v interface{}) interface{}`** — recursively normalises the output of `go-yaml` unmarshalling so that it is compatible with gojq:
- Converts `map[interface{}]interface{}` (produced by go-yaml v2 / yaml.in) to `map[string]interface{}`.
- Converts `time.Time` values (produced by go-yaml for timestamp nodes) to RFC 3339 strings, because gojq cannot process `time.Time` natively.

**`YAMLCheckExist(yamlData []byte, query string) (bool, error)`** — convenience function that unmarshals YAML bytes, normalises the result, runs a jq query expected to return a boolean, and returns `true`/`false`. Useful for asserting that a specific key or value exists in a YAML document.

---

## Usage

**`pkg/compliance/resolver.go`** — uses `RunSingleOutput` to evaluate jq expressions defined in compliance rules against JSON payloads collected from the host (e.g. process metadata, file contents). This allows rule authors to use jq syntax to navigate and extract fields from arbitrary JSON.

**`cmd/agent/common/autodiscovery.go`** — uses the package to evaluate filter expressions against container/pod annotations serialised as JSON during autodiscovery template resolution.

**`comp/core/agenttelemetry/impl/agenttelemetry_test.go`** — uses `RunSingleOutput` in tests to assert the structure of JSON telemetry payloads.

### Cache interaction

The query cache uses `pkg/util/cache` under the `"jq-<q>"` key with a 6-hour TTL. When the same jq expression string appears in many calls (e.g. compliance rules evaluated on every check cycle), the cache eliminates repeated `gojq.Parse` + `gojq.Compile` calls. Cache misses are still fast for short jq programs but the caching matters on high-frequency paths.

### Choosing between `RunSingleOutput` and raw `gojq`

Use `RunSingleOutput` when:
- Only the first output value is needed.
- The result is always representable as a string (numbers, booleans, and strings are all formatted with `fmt.Sprintf`).

Use `gojq` directly (via `Parse`) when:
- Multiple results must be iterated (`code.Run(v).Next()` loop).
- You need typed results other than `string`.

### YAML normalization notes

`NormalizeYAMLForGoJQ` must be called before running a gojq query against any value produced by
`go-yaml` (v2/v3). The two specific transformations it handles are:

1. `map[interface{}]interface{}` → `map[string]interface{}` — go-yaml v2 produces non-string map keys that gojq refuses.
2. `time.Time` → RFC 3339 string — go-yaml parses ISO timestamp nodes natively; gojq cannot compare or format them.

Config YAML loaded through `pkg/config` is already decoded into typed structs, so `NormalizeYAMLForGoJQ` is most useful when you unmarshal raw YAML bytes yourself (e.g. reading a check configuration file, an annotation value, or a compliance rule payload).

### Example

```go
// Check whether a YAML config file enables a feature
enabled, err := jsonquery.YAMLCheckExist(configBytes, `.network_config.enabled`)

// Extract a field from a JSON object
val, found, err := jsonquery.RunSingleOutput(`.hostname`, payloadMap)
```

## Cross-references

| Document | Relationship |
|---|---|
| [`pkg/util/json`](json.md) | Lightweight alternative for simple key traversal of `map[string]interface{}` trees when jq expressions are not needed |
| [`pkg/config`](../config/config.md) | Agent configuration system; config values are typed structs so `NormalizeYAMLForGoJQ` is typically only needed when processing raw YAML outside of `pkg/config` |
