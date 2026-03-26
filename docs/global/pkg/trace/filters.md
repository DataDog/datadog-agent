> **TL;DR:** `pkg/trace/filters` provides two complementary data-control primitives: `Blacklister` drops entire traces whose resource names match deny-list regex patterns, and `Replacer` scrubs sensitive tag values in-place via configurable regex replacement rules.

# pkg/trace/filters

## Purpose

Provides two complementary filtering primitives used by the trace agent to control what data is forwarded to Datadog:

- **Blocking** — drop entire spans/stats buckets whose resource name matches a deny-list pattern.
- **Scrubbing** — rewrite tag values in-place using find-and-replace rules without dropping spans.

## Key elements

### `Blacklister` (`blacklister.go`)

```go
type Blacklister struct { ... }

func NewBlacklister(exprs []string) *Blacklister
func (f *Blacklister) Allows(span *pb.Span) (bool, *regexp.Regexp)
func (f *Blacklister) AllowsString(s string) (bool, *regexp.Regexp)
func (f *Blacklister) AllowsStat(stat *pb.ClientGroupedStats) bool
```

Holds a compiled list of regular expressions matched against a span's `Resource` field (or an arbitrary string). Returns `(false, matchingRule)` when a span should be dropped. Regex compilation errors are logged and the offending pattern is silently skipped.

Configured from `conf.Ignore["resource"]` — a list of resource-name patterns for which the agent should not forward data.

### `Replacer` (`replacer.go`)

```go
type Replacer struct { ... }

func NewReplacer(rules []*config.ReplaceRule) *Replacer
func (f Replacer) Replace(trace pb.Trace)
func (f Replacer) ReplaceV1(trace *idx.InternalTraceChunk)
func (f Replacer) ReplaceStatsGroup(b *pb.ClientGroupedStats)
```

Applies a list of `ReplaceRule` entries to every span in a trace. Each rule specifies:
- `Name` — the tag key to target (`"*"` for all user-visible tags, `"resource.name"` for the resource field, or an exact tag name).
- `Re` — a compiled `*regexp.Regexp` to match against the tag's current value.
- `Repl` — the replacement string (supports regex back-references).

Internal tags whose names start with `_` are never rewritten. When a numeric metric tag is replaced by a non-numeric string, the value is moved from `Metrics` to `Meta`. `ReplaceV1` handles the string-interned `idx.InternalTraceChunk` representation used by the v1 ingestion path. `ReplaceStatsGroup` applies resource-name and HTTP status code replacement to pre-aggregated stats.

Configured from `conf.ReplaceTags`.

## Usage

Both types are created once in `agent.NewAgent` and stored on the `Agent` struct:

```go
Blacklister: filters.NewBlacklister(conf.Ignore["resource"]),
Replacer:    filters.NewReplacer(conf.ReplaceTags),
```

During trace processing the agent checks the deny-list first:

```go
if allowed, denyingRule := a.Blacklister.Allows(root); !allowed {
    // drop the entire trace chunk
}
```

Then rewrites tags on surviving spans:

```go
a.Replacer.Replace(chunk.Spans)
```

The same pattern applies in the stats pipeline (`AllowsStat` / `ReplaceStatsGroup`) so that blocked or scrubbed data is consistent between trace payloads and pre-aggregated metrics.
