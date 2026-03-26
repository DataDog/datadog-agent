> **TL;DR:** `pkg/obfuscate` strips or redacts sensitive values from database queries, cache commands, HTTP URLs, and JSON before they reach Datadog as span resource names or tags, and is published as a standalone Go module so it can be shared by the trace agent, `dd-trace-go`, and the OTel Collector Datadog exporter.

# Package `pkg/obfuscate`

## Purpose

`pkg/obfuscate` strips or redacts sensitive values from database queries, cache
commands, HTTP URLs, and arbitrary JSON before they are stored in Datadog as
span resource names or tags. It also scans span metadata for credit card
numbers and removes them.

The package operates on raw string inputs (query text, command strings, JSON
bodies) and returns sanitized strings. It is intentionally self-contained: it
lives in its own Go module (`github.com/DataDog/datadog-agent/pkg/obfuscate`)
so that it can be imported by:

- the Datadog Trace Agent (`pkg/trace/agent/`),
- the Go tracing client (`dd-trace-go`),
- the OpenTelemetry Collector Datadog exporter.

The public Go API has no stability guarantees but breaking changes should be
minimized because of the shared external consumers.

---

## Key elements

### Key types

| Type | Description |
|---|---|
| `Obfuscator` | Central struct. Holds per-dialect obfuscators (SQL, MongoDB/ES/OpenSearch JSON, credit cards) plus a shared LRU query cache. Not safe for concurrent use without external locking. |

### Key functions

| Element | Description |
|---|---|
| `NewObfuscator(cfg Config) *Obfuscator` | Creates an `Obfuscator`. Lazily enables per-dialect sub-obfuscators based on the flags in `cfg`. |
| `(*Obfuscator) Stop()` | Shuts down the background cache stats goroutine. Call when the obfuscator is no longer needed. |
| `Version` constant (= `1`) | Monotonically incrementing integer. Clients can embed this in cache keys so that stale cached results from an older obfuscation logic version are not reused. |

### Configuration and build flags

`Config` is the top-level configuration struct passed to `NewObfuscator`. All
fields are optional; zero values disable the corresponding feature.

| Field | Type | Description |
|---|---|---|
| `SQL` | `SQLConfig` | SQL obfuscation and normalization options. |
| `ES` | `JSONConfig` | Elasticsearch request body obfuscation. |
| `OpenSearch` | `JSONConfig` | OpenSearch request body obfuscation. |
| `Mongo` | `JSONConfig` | MongoDB query body obfuscation. |
| `SQLExecPlan` | `JSONConfig` | SQL execution plan obfuscation (safety only, no normalization). |
| `SQLExecPlanNormalize` | `JSONConfig` | SQL execution plan normalization. |
| `HTTP` | `HTTPConfig` | HTTP URL query-string and path-digit removal. |
| `Redis` | `RedisConfig` | Redis command argument obfuscation. |
| `Valkey` | `ValkeyConfig` | Valkey command argument obfuscation (same structure as Redis). |
| `Memcached` | `MemcachedConfig` | Memcached command obfuscation. |
| `CreditCard` | `CreditCardsConfig` | Credit card number detection in span metadata values. |
| `Cache` | `CacheConfig` | LRU cache for obfuscated SQL/MongoDB queries. Enable with `Enabled: true` and set `MaxSize` (bytes). |
| `Statsd` | `StatsClient` | Optional stats client for cache hit/miss gauges (`datadog.trace_agent.obfuscation.sql_cache.*`). |
| `Logger` / `FullLogger` | `Logger` / `FullLogger` | Optional loggers. `FullLogger` takes precedence. If neither is set, all log output is suppressed. |

#### `SQLConfig` notable fields

| Field | Description |
|---|---|
| `ObfuscationMode` | One of `NormalizeOnly`, `ObfuscateOnly`, `ObfuscateAndNormalize`. Selects the underlying engine (legacy tokenizer vs. `go-sqllexer`). |
| `DBMS` | Database type (e.g. `"postgresql"`, `"mysql"`). Used by `go-sqllexer` for dialect-aware parsing. |
| `TableNames` | Extract referenced table names into `SQLMetadata.TablesCSV`. |
| `CollectCommands` | Extract SQL commands (`SELECT`, `UPDATE`, …) into `SQLMetadata.Commands`. |
| `CollectComments` | Extract inline comments into `SQLMetadata.Comments`. |
| `ReplaceDigits` | Replace digit sequences in table/identifier names with `?`. |
| `KeepSQLAlias` | Retain `AS` aliases rather than stripping them. |
| `DollarQuotedFunc` | Treat PostgreSQL `$func$…$func$` dollar-quoted strings as code, not literals. |
| `KeepNull` / `KeepBoolean` / `KeepPositionalParameter` | Suppress obfuscation of NULL, boolean, and positional (`$1`) literals when using `go-sqllexer`. |
| `Cache` | Deprecated per-SQL cache toggle. Use `Config.Cache` instead. |

#### `JSONConfig` notable fields

| Field | Description |
|---|---|
| `Enabled` | Must be `true` to activate JSON obfuscation for this dialect. |
| `KeepValues` | Keys whose values should not be redacted. |
| `ObfuscateSQLValues` | Keys whose values should be treated as SQL strings and run through SQL obfuscation. |

### Key interfaces

(No exported interfaces. The package is used entirely through the `*Obfuscator` concrete type and the `StatsClient` and `Logger` input interfaces documented under Configuration.)

### Public obfuscation methods

| Method | Description |
|---|---|
| `ObfuscateSQLString(in string) (*ObfuscatedQuery, error)` | Obfuscates and/or normalizes `in` using the options in `Config.SQL`. Results are cached when the cache is enabled. |
| `ObfuscateSQLStringForDBMS(in, dbms string) (*ObfuscatedQuery, error)` | Same as above but overrides the `DBMS` field for dialect-aware `go-sqllexer` obfuscation. |
| `ObfuscateSQLStringWithOptions(in string, opts *SQLConfig, optsStr string) (*ObfuscatedQuery, error)` | Full control over SQL options; `optsStr` is the JSON-marshalled representation of `opts` used as the cache key discriminator. |
| `ObfuscateWithSQLLexer(in string, opts *SQLConfig) (*ObfuscatedQuery, error)` | Directly invokes the `go-sqllexer` path regardless of `ObfuscationMode`. |
| `ObfuscateSQLExecPlan(jsonPlan string, normalize bool) (string, error)` | Obfuscates (and optionally normalizes) a SQL execution plan encoded as JSON. |
| `ObfuscateMongoDBString(cmd string) string` | Redacts sensitive values in a MongoDB query JSON document. Returns `""` if the JSON obfuscator is disabled. |
| `ObfuscateElasticSearchString(cmd string) string` | Same for Elasticsearch request bodies. |
| `ObfuscateOpenSearchString(cmd string) string` | Same for OpenSearch request bodies. |
| `QuantizeRedisString(query string) string` | Strips all arguments and returns only the command names (up to 3, followed by `...` if truncated). Does not use the tokenizer; prefer `ObfuscateRedisString` for more precise argument-level redaction. |
| `ObfuscateRedisString(rediscmd string) string` | Tokenizes `rediscmd` and redacts only the specific arguments that carry sensitive data for each known Redis command (e.g. `SET key ?`, `AUTH ?`). |
| `RemoveAllRedisArgs(rediscmd string) string` | Replaces all arguments with a single `?`. Used when `RedisConfig.RemoveAllArgs` is set. |
| `ObfuscateURLString(val string) string` | Removes user-info from URLs. Optionally strips query strings and replaces path segments containing digits with `?`. |
| `ShouldObfuscateCCKey(key string) bool` | Returns true if a span metadata key is a candidate for credit card scanning. |
| `ObfuscateCreditCardNumber(val string) string` | Returns `"?"` if `val` looks like a credit card number (with optional Luhn validation). Otherwise returns `val` unchanged. |

### Return types

| Type | Description |
|---|---|
| `ObfuscatedQuery` | Returned by SQL obfuscation methods. `Query string` holds the sanitized SQL; `Metadata SQLMetadata` carries optional extracted metadata (tables, commands, comments, procedures). |
| `SQLMetadata` | `TablesCSV string`, `Commands []string`, `Comments []string`, `Procedures []string`, `Size int64` (byte size of collected metadata). |

### `ObfuscationMode` constants

| Constant | Value | Description |
|---|---|---|
| `NormalizeOnly` | `"normalize_only"` | Whitespace normalization only; no literal replacement. |
| `ObfuscateOnly` | `"obfuscate_only"` | Replace literals with `?` but do not normalize whitespace/structure. |
| `ObfuscateAndNormalize` | `"obfuscate_and_normalize"` | Both operations in one pass (recommended default). |

### Caching

The LRU cache (backed by `ristretto`) is keyed on `hash(query) XOR hash(sqlOptsStr)` and stores `*ObfuscatedQuery` values. Cache cost is computed via `ObfuscatedQuery.Cost()` which accounts for the size of the query string plus collected metadata (roughly `len(query) + metadata size + 320 bytes` fixed overhead). Cache hit/miss rates are emitted every 10 seconds via the configured `StatsClient`.

---

## Usage

### How it is used in the codebase

The primary consumer is **`pkg/trace/agent/obfuscate.go`**, which creates one
`Obfuscator` per trace agent instance (configured from `apm_config.obfuscation`
in `datadog.yaml`) and calls the appropriate method on every span based on the
span type (e.g. `sql` → `ObfuscateSQLString`, `redis` → `ObfuscateRedisString`
or `QuantizeRedisString`, `mongodb` → `ObfuscateMongoDBString`, etc.).

The obfuscator is also used in:
- `pkg/trace/otel/stats/otel_util.go` — obfuscates resource names extracted
  from OTel spans before computing stats.
- `pkg/trace/transform/` — applies obfuscation during span transformation
  for non-standard ingestion paths.

### Typical initialization

```go
import "github.com/DataDog/datadog-agent/pkg/obfuscate"

o := obfuscate.NewObfuscator(obfuscate.Config{
    SQL: obfuscate.SQLConfig{
        ObfuscationMode: obfuscate.ObfuscateAndNormalize,
        TableNames:      true,
        CollectCommands: true,
    },
    Redis: obfuscate.RedisConfig{Enabled: true},
    HTTP:  obfuscate.HTTPConfig{RemoveQueryString: true},
    Cache: obfuscate.CacheConfig{Enabled: true, MaxSize: 50_000_000},
})
defer o.Stop()

oq, err := o.ObfuscateSQLString("SELECT * FROM users WHERE id = 42")
// oq.Query      -> "SELECT * FROM users WHERE id = ?"
// oq.Metadata.TablesCSV -> "users"
// oq.Metadata.Commands  -> ["SELECT"]
```

### Configuration in `datadog.yaml`

All obfuscation options are nested under `apm_config.obfuscation`:

```yaml
apm_config:
  obfuscation:
    elasticsearch:
      enabled: true
      keep_values: ["role"]
    mongodb:
      enabled: true
    http:
      remove_query_string: true
      remove_paths_with_digits: true
    redis:
      enabled: true
      remove_all_args: false
    memcached:
      enabled: true
    credit_cards:
      enabled: true
      luhn: true
    cache:
      enabled: true
      max_size: 50000000
```

SQL obfuscation is always on by default; the mode and metadata collection
options are controlled by individual fields within `apm_config.obfuscation.sql`.

### Important caveats

- `Obfuscator` is **not safe for concurrent use**. The trace agent serializes
  all obfuscation calls through a single goroutine.
- SQL literal escape handling is auto-detected: the first query whose
  obfuscation reveals a literal-escape mismatch sets `sqlLiteralEscapes` via an
  `atomic.Bool`, affecting all subsequent queries.
- The package is a standalone Go module. If you need to import it outside the
  main agent module, add it as a direct dependency in your `go.mod`.

---

## Cross-references

| Topic | Document |
|---|---|
| Trace agent pipeline that calls the obfuscator (step 4 in the processing chain) | [`pkg/trace`](trace/trace.md) |
| `AgentConfig.Obfuscation` and `ObfuscationConfig.Export(conf)` — how YAML config is translated into `obfuscate.Config` | [`pkg/trace/config`](trace/config.md) |
| `apm_config.replace_tags` regex-based PII scrubbing applied **after** obfuscation | [`pkg/trace/filters`](trace/filters.md) |
| `pkg/redact` — command-line and Kubernetes manifest scrubbing (process/orchestrator pipeline, not APM) | [`pkg/redact`](redact.md) |

### How `pkg/trace/config` bridges to this package

`ObfuscationConfig.Export(conf)` (in `pkg/trace/config`) converts the agent's
`apm_config.obfuscation.*` YAML section into an `obfuscate.Config` value and
passes it to `NewObfuscator`. Feature flags in `AgentConfig.Features` (e.g.,
`"sqllexer"`) are inspected inside `Export` to fine-tune `SQLConfig.ObfuscationMode`
before the `Obfuscator` is constructed.
