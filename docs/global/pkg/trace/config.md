> **TL;DR:** `pkg/trace/config` defines `AgentConfig`, the single shared configuration object for the entire trace agent, along with all sub-structures and defaults; every other `pkg/trace/*` package receives an `*AgentConfig` pointer at construction time and never reads `datadog.yaml` directly after startup.

# pkg/trace/config

## Purpose

`pkg/trace/config` defines the central configuration structure for the trace
agent — `AgentConfig` — and all the sub-structures it depends on. Every other
`pkg/trace/*` package receives an `*AgentConfig` pointer at construction time;
nothing reads `datadog.yaml` directly after startup. The package also provides
`ResetClient`, a thin `http.Client` wrapper that periodically resets idle TCP
connections, and helpers for peer-tag aggregation configuration.

## Key elements

### AgentConfig

`AgentConfig` is the single shared config object for the entire trace agent.
It is created with `New()` (which applies all defaults) and then populated by
`cmd/trace-agent` from `datadog.yaml` / environment variables.

Selected fields grouped by area:

**Identity & routing**
- `Endpoints []*Endpoint` — list of `{Host, APIKey}` pairs. `Endpoints[0]` is
  the primary intake. Additional entries come from `additional_endpoints` in
  config and support Multi-Region Failover (`IsMRF` flag).
- `Site string` — e.g. `"datadoghq.com"`. Used to derive default intake URLs.
- `DefaultEnv string` — env tag applied to traces that don't carry their own.
- `Hostname string` — agent hostname reported to the backend.
- `Features map[string]struct{}` — runtime feature flags (e.g. `"sqllexer"`,
  `"disable_cid_stats"`, `"probabilistic_sampler_full_trace_id"`). Check with
  `HasFeature(name)`.

**Sampler settings**

| Field | Default | Description |
|-------|---------|-------------|
| `TargetTPS` | 10 | Target traces-per-second for `PrioritySampler`. |
| `ErrorTPS` | 10 | TPS budget for `ErrorsSampler`. |
| `MaxEPS` | 200 | Max APM events per second. |
| `ExtraSampleRate` | 1.0 | Multiplicative rate applied on top of computed rates. |
| `RareSamplerEnabled` | false | Whether `RareSampler` is active. |
| `RareSamplerTPS` | 5 | Token-bucket rate limit for rare traces. |
| `RareSamplerCooldownPeriod` | 5 min | TTL before a rare span signature can be sampled again. |
| `RareSamplerCardinality` | 200 | Max unique span signatures per (env, service) shard. |
| `ProbabilisticSamplerEnabled` | false | Enables `ProbabilisticSampler`. |
| `ProbabilisticSamplerSamplingPercentage` | — | 0–100 percentage of traces to keep. |
| `ProbabilisticSamplerHashSeed` | — | FNV seed for deterministic hashing. |

**Stats / Concentrator settings**
- `BucketInterval time.Duration` (default 10 s) — stats bucket width.
- `PeerTagsAggregation bool` (default true) — enables peer entity tag
  dimensions in stats aggregation.
- `ComputeStatsBySpanKind bool` (default true) — includes server/client/
  consumer/producer spans in stats even without `_top_level`.
- `PeerTags []string` — extra tag keys to aggregate on (appended to base set
  from `peer_tags.ini`).
- `SpanDerivedPrimaryTagKeys []string` — tag keys that become extra aggregation
  dimensions in stats.

**Writer settings**
- `TraceWriter *WriterConfig`, `StatsWriter *WriterConfig` — per-writer tuning:
  `ConnectionLimit`, `QueueSize`, `FlushPeriodSeconds`.
- `SynchronousFlushing bool` — serverless mode; writers only flush when
  `FlushSync()` is called.
- `MaxSenderRetries int` (default 4) — max HTTP retry attempts per payload.
- `ConnectionResetInterval time.Duration` — how often `ResetClient` recreates
  its underlying `http.Client` to close idle connections.
- `ClientStatsFlushInterval time.Duration` (default 2 s) — flush interval for
  `ClientStatsAggregator`.

**Receiver settings**
- `ReceiverPort int` (default 8126) — HTTP port for tracer payloads.
- `ReceiverSocket string` — Unix domain socket path (optional).
- `MaxRequestBytes int64` (default 25 MB) — payload size limit.

**Obfuscation**
- `Obfuscation *ObfuscationConfig` — per-technology sensitive data scrubbing
  config (SQL, Redis, Mongo, ElasticSearch, HTTP URLs, credit cards, etc.).
- `SQLObfuscationMode string` — overrides the SQL obfuscator mode
  (`"obfuscate_only"`, `"normalize_only"`, `"obfuscate_and_normalize"`).

**Filtering**
- `Ignore map[string][]string` — resource name blocklist keyed by tag name.
- `ReplaceTags []*ReplaceRule` — regex-based tag value redaction.
- `RequireTags / RejectTags []*Tag` — root-span tag allow/deny lists.
- `MaxResourceLen int` (default 5,000) — resource name truncation limit.

**Infrastructure**
- `ContainerTags func(cid string) ([]string, error)` — injected by the
  component framework to resolve container tags from a container ID.
- `MRFFailoverAPMRC *bool` — remote-config override for MRF APM failover.
- `RemoteConfigClient RemoteClient` — interface for receiving remote sampling
  updates.

### Supporting types

| Type | Description |
|------|-------------|
| `Endpoint` | `{Host, APIKey, NoProxy, IsMRF}`. The first entry is primary; others are dual-write or MRF endpoints. |
| `WriterConfig` | `{ConnectionLimit, QueueSize, FlushPeriodSeconds}` — per-writer HTTP settings. |
| `ObfuscationConfig` | Nested config structs for each obfuscation target. Call `Export(conf)` to produce an `obfuscate.Config`. |
| `ReplaceRule` | `{Name, Pattern, Re, Repl}` — one regex replacement rule for a tag. |
| `OTLP` | OpenTelemetry receiver settings (gRPC port, span name remappings, probabilistic sampling percentage). |
| `ResetClient` | `http.Client` wrapper (`NewResetClient(interval, factory)`) that calls `CloseIdleConnections()` and recreates the client at `interval`. Used by sender HTTP connections. |
| `RemoteClient` interface | `Subscribe`, `UpdateApplyStatus`, `Start`, `Close` — abstraction over `pkg/config/remote` for testability. |
| `Tag`, `TagRegex` | Simple `{K, V}` pair and its regex variant for `RequireTags`/`RejectTags` filtering. |

### Key functions

| Function | Description |
|----------|-------------|
| `New()` | Returns `*AgentConfig` with all production defaults. Always use this as the base. |
| `(c *AgentConfig) HasFeature(feat)` | Reports whether a feature flag is active. |
| `(c *AgentConfig) APIKey()` | Returns `Endpoints[0].APIKey`. |
| `(c *AgentConfig) NewHTTPClient()` | Creates a `*ResetClient` backed by `NewHTTPTransport()`. Used by senders. |
| `(c *AgentConfig) ConfiguredPeerTags()` | Returns the resolved peer tag list (base set from `peer_tags.ini` merged with `PeerTags`), or nil if `PeerTagsAggregation` is false. |
| `(c *AgentConfig) ConfiguredSpanDerivedPrimaryTagKeys()` | Returns deduplicated, sorted span-derived tag keys. |
| `(c *AgentConfig) MRFFailoverAPM()` | Returns the effective MRF failover flag (remote config overrides static config). |
| `(o *ObfuscationConfig) Export(conf)` | Converts `ObfuscationConfig` to the `obfuscate.Config` struct consumed by `pkg/obfuscate`. Reads feature flags from `conf` to fine-tune SQL behaviour. |

### Peer tags

`peer_tags.ini` (embedded via `//go:embed`) ships a curated list of tag keys
(e.g. `peer.service`, `db.system`, `messaging.system`) that the agent uses for
peer entity stats aggregation. `ConfiguredPeerTags()` merges this list with any
user-defined `PeerTags` and deduplicates/sorts the result.

## Usage

`pkg/trace/config` is used as a pure data package — nothing in this directory
starts goroutines or owns resources (except `ResetClient`).

```go
// Typical startup in cmd/trace-agent
conf := config.New()       // apply defaults
setupConfig(conf, cfgPath) // populate from datadog.yaml / env vars
// pass conf to all sub-components:
agent := agent.NewAgent(ctx, conf, ...)
```

Tests typically call `config.New()` and mutate specific fields rather than
building configs from scratch, ensuring they always inherit correct defaults.
