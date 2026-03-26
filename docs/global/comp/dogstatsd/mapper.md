# comp/dogstatsd/mapper — Metric Name Mapping Component

**Import path:** `github.com/DataDog/datadog-agent/comp/dogstatsd/mapper`
**Team:** agent-metric-pipelines
**Importers:** `comp/dogstatsd/server`

## Purpose

`comp/dogstatsd/mapper` translates raw DogStatsD metric names into structured Datadog metric names and tags before the samples reach the aggregator. This is useful when clients emit StatsD metrics with positional information encoded in the name (e.g. `api.latency.us-east.200`) and you want to extract those segments as tags instead of encoding them into the metric name.

The feature is inspired by the [Prometheus statsd_exporter](https://github.com/prometheus/statsd_exporter) mapping syntax.

Mapping is configured via `dogstatsd_mapper_profiles` in `datadog.yaml`. Each profile targets a prefix and defines an ordered list of rules. The mapper tries rules in order and returns the first match. Results are cached in an LRU cache to avoid repeated regex evaluation on hot metric names.

## Package layout

This package has no `def/` or `impl/` split — all types and logic live directly in `comp/dogstatsd/mapper`.

| File | Role |
|---|---|
| `mapper.go` | `MetricMapper`, `MappingProfile`, `MetricMapping`, `MapResult`, `NewMetricMapper`, `Map` |
| `mapper_cache.go` | `mapperCache` — LRU wrapper around `*MapResult` keyed by metric name |
| `component.go` | (empty — package is not an fx component; consumed directly by the server) |
| `component_mock.go` | Mock helper for tests |

## Key types

### `MappingProfileConfig` and `MetricMappingConfig`

Used to deserialise `dogstatsd_mapper_profiles` from the agent config (via `structure.UnmarshalKey`):

```go
type MappingProfileConfig struct {
    Name     string                // profile name (required)
    Prefix   string                // metric name prefix this profile applies to (required)
    Mappings []MetricMappingConfig
}

type MetricMappingConfig struct {
    Match     string            // pattern (wildcard or regex)
    MatchType string            // "wildcard" (default) or "regex"
    Name      string            // output metric name; capture groups via $1, $2 or named groups
    Tags      map[string]string // tag key -> value expression (capture group references allowed)
}
```

### `MetricMapper`

```go
type MetricMapper struct {
    Profiles []MappingProfile
    cache    *mapperCache
}

func NewMetricMapper(configProfiles []MappingProfileConfig, cacheSize int) (*MetricMapper, error)
func (m *MetricMapper) Map(metricName string) *MapResult
```

`NewMetricMapper` validates all profiles and rules, compiles wildcard patterns to regular expressions, and allocates the LRU cache. Returns an error if a profile is missing a name or prefix, a rule is missing `match` or `name`, or a pattern fails to compile.

`Map` returns a `*MapResult` for the first matching rule, or `nil` if no rule matches. Results (including non-matches) are cached to avoid recompiling on repeated metric names.

### `MapResult`

```go
type MapResult struct {
    Name    string   // translated metric name
    Tags    []string // "key:value" tag strings
    matched bool     // unexported; indicates a successful match
}
```

### `mapperCache`

An LRU cache (using `github.com/hashicorp/golang-lru/v2`) keyed on the original metric name. Both match and non-match results are cached. Cache size is configurable (passed as `cacheSize` to `NewMetricMapper`).

## Wildcard vs. regex matching

**Wildcard** (default): `*` matches any sequence of characters that does not contain `.`. Each `*` becomes a capture group in the compiled regex. Example:

```yaml
match: "api.latency.*.*"
name: "api.latency"
tags:
  region: "$1"
  status_code: "$2"
```

Constraints: only `[a-zA-Z0-9\-_*.]` characters are allowed; consecutive `**` is rejected.

**Regex**: the `match` value is used as-is as a regular expression. Capture groups can be referenced in `name` and tag values.

```yaml
match_type: regex
match: "api\.latency\.(?P<region>[^.]+)\.(?P<status>\d+)"
name: "api.latency"
tags:
  region: "${region}"
  status_code: "${status}"
```

## Usage in the codebase

The mapper is used exclusively by `comp/dogstatsd/server`. At startup the server reads `dogstatsd_mapper_profiles` and constructs a `*MetricMapper`:

```go
mappings, err := getDogstatsdMappingProfiles(s.config)
mapperInstance, err := mapper.NewMetricMapper(mappings, cacheSize)
s.mapper = mapperInstance
```

During metric parsing the server calls `Map` on each sample name before enrichment:

```go
if s.mapper != nil {
    mapResult := s.mapper.Map(sample.name)
    if mapResult != nil {
        sample.name = mapResult.Name
        sample.tags = append(sample.tags, mapResult.Tags...)
    }
}
```

When `Map` returns `nil` (no match) the original metric name is preserved unchanged.

## Configuration example

```yaml
dogstatsd_mapper_profiles:
  - name: my_service
    prefix: "my_service."
    mappings:
      - match: "my_service.http.request.*.*"
        name: "my_service.http.request"
        tags:
          http_method: "$1"
          http_code:   "$2"
      - match: "my_service.cache.*"
        name: "my_service.cache.operations"
        tags:
          cache_name: "$1"
```

## Related components

| Component | Doc | Relationship |
|---|---|---|
| `comp/dogstatsd/server` | [server.md](server.md) | The only consumer. Constructs `*MetricMapper` from `dogstatsd_mapper_profiles` at startup and calls `Map` on every raw metric name during the parse/enrich phase. |
| `comp/core/config` | [../core/config.md](../core/config.md) | The server reads `dogstatsd_mapper_profiles` via `config.Component` using `structure.UnmarshalKey`, which deserialises the YAML into `[]MappingProfileConfig`. The mapper itself does not hold a reference to `config.Component`. |

## Usage patterns (extended)

### Constructing a mapper in a unit test

```go
import "github.com/DataDog/datadog-agent/comp/dogstatsd/mapper"

profiles := []mapper.MappingProfileConfig{
    {
        Name:   "test",
        Prefix: "test.",
        Mappings: []mapper.MetricMappingConfig{
            {
                Match: "test.latency.*.*",
                Name:  "test.latency",
                Tags:  map[string]string{"env": "$1", "service": "$2"},
            },
        },
    },
}

m, err := mapper.NewMetricMapper(profiles, 1000 /*cacheSize*/)
if err != nil { ... }

result := m.Map("test.latency.prod.api")
// result.Name == "test.latency"
// result.Tags == ["env:prod", "service:api"]
```

### Cache considerations

The LRU cache is keyed on the **exact original metric name** before any transformation. Both hit and miss results are cached — a `nil` miss result prevents repeated regex evaluation for names that never match. Choose a `cacheSize` that covers the cardinality of unique metric names your clients emit; the default in the server is set by `dogstatsd_mapper_cache_size` (default `1000`).

### Adding a new profile at runtime

The mapper is constructed once at server startup and stored in `server.mapper`. To apply new profiles, the agent must be restarted (there is no hot-reload path for mapper profiles). Use `datadog-agent config set` to persist other `dogstatsd_*` settings, but mapper profile changes require editing `datadog.yaml` and restarting.
