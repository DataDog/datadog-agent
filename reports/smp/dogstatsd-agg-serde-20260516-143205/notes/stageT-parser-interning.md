# Stage T — parser string/tagset interning

Goal: attack the dominant parser-side allocation identified in Stage S. The late Stage R Agent profile showed `serverimpl.(*stringInterner).LoadOrStore` at roughly `17.64 GiB` alloc-space over the standard-UDS profiling run, and `parseTags` allocated another `~2.40 GiB` for per-message `[]string` tag slices.

## Profile signal

From the Stage S standard v3 UDS capture (`profiles/stageR-agent-heap/captures-main-vs-stageR-pprof-standard/comparison/replicate-0/captures.parquet`):

| Metric | interner_0 max | interner_1 max |
|---|---:|---:|
| `dogstatsd.string_interner_hits` | `59,747,663` | `59,757,629` |
| `dogstatsd.string_interner_miss` | `52,988,744` | `52,985,848` |
| `dogstatsd.string_interner_resets` | `12,936` | `12,936` |
| `dogstatsd.string_interner_entries` | `4,081` | `4,094` |
| `dogstatsd.string_interner_bytes` | `314,229` | `317,947` |

Read: the default `dogstatsd_string_interner_size=4096` cache was constantly resetting. Resets kept live interner bytes bounded, but repeatedly discarded hot names/tags and forced the parser to reallocate strings that are likely reused.

## Code changes

### 1. Replace full-reset string interner with bounded SLRU

The parser string interner now uses two bounded segments:

- `recent` / probationary: first sightings enter here;
- `protected`: a hit in `recent` promotes the string;
- both segments use ring eviction;
- no whole-cache reset on high-cardinality churn.

This keeps the same no-allocation hit trick (`map[string]...` lookup with `string([]byte)` directly in the index expression), but avoids the old cliff where one miss at capacity threw away every hot entry.

Telemetry:

- existing hit/miss/size/bytes gauges remain;
- `dogstatsd.string_interner_resets` should stop increasing for the new path;
- new `dogstatsd.string_interner_evictions` records individual bounded evictions.

### 2. Make tag extraction non-mutating for normal tags

`extractTagsMetadata` now only mutates the parsed tag slice if it actually removes metadata tags (`host:`, `dd.internal.entity_id:`, `dd.internal.card:`, `dd.internal.jmx_check_name:`). Normal client tagsets are treated as immutable. This is a prerequisite for safely sharing parsed tagset slices.

### 3. Add optional exact raw-tagset interner

Experimental env gates:

```bash
DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true
DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER_SIZE=<entries>
```

Design:

- exact raw tagset cache, analogous to a v3 payload-local tagset dictionary;
- hit path uses `map[string][]string` lookup with `string(rawTags)` directly in the lookup expression, so exact hits do not allocate;
- miss path uses a small recent hash doorkeeper: first sighting records a hash but does not retain the whole raw tagset key; second sighting admits the parsed tagset;
- tagsets containing metadata tags are not admitted, because those may need compatibility projection/removal;
- hot telemetry on every hit/miss was deliberately avoided after a benchmark showed per-hit telemetry reintroduced allocation. The cache currently keeps local counters only; macro validation should use existing parser/interner allocation profiles and DogStatsD throughput/RSS metrics.

## Focused benchmark

Command:

```bash
dda inv test --targets=./comp/dogstatsd/server/impl \
  --test-run-name='^$' \
  --extra-args='-bench=BenchmarkParseTagsRepeatedTagset -benchmem -benchtime=3s' \
  --timeout=300
```

Result after removing hot telemetry from the tagset cache:

| Path | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| default parseTags repeated tagset | `145.9` | `112` | `1` |
| exact tagset interner hit | `15.64` | `0` | `0` |

This is a best-case repeated exact tagset microbenchmark, not a macro claim. It proves the in-memory v3-style tagset dictionary can eliminate the per-message `[]string` allocation and split work when exact tagsets repeat.

## Validation

```bash
dda inv test --targets=./comp/dogstatsd/server/impl,./comp/dogstatsd/listeners,./comp/dogstatsd/packets --timeout=300
```

Result: passed (`318` tests).

## SMP validation

Images:

| Image | ID | Notes |
|---|---|---|
| `datadog/agent-dev:smp-dsd-main` | `sha256:6f67e85689c833453bb60ba0697d234561a39a9f8df37d36f2a7fd6372316419` | baseline, commit `3ec880f14a3` |
| `datadog/agent-dev:smp-dsd-columnar-v3-parser-interning` | `sha256:9bce78118472178a9d815f9bd7a8e50c80b46d4cf92f502969a2b29d66ab3012` | Stage T default, commit `d78e87470fd` |
| `datadog/agent-dev:smp-dsd-columnar-v3-parser-interning-tagset` | `sha256:99c37921c6bed2145e1a848499a8e83885f5ee5bfff23601da0f47890c00b93b` | same Stage T image with `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true` baked into image env |

Build note: the final Stage T image was built from a Linux/arm64 dev container with `--no-development`. A local untracked `.dockerignore` was used to keep `.git`, `reports/`, and captures out of the Docker context; otherwise `COPY .` in `agent.hacky-dev-image-build` sent a multi-GB context and previously exhausted the Colima disk.

Three-replicate SMP results (`--total-samples 270`):

| Comparison | Case | Δ mean | 95% CI | Read |
|---|---|---:|---:|---|
| Stage T default vs `main` | standard v3 UDS | `+2.98%` | `[+2.60%, +3.36%]` | improvement |
| Stage T default vs `main` | high-rate v3 UDS metrics-only | `+4.88%` | `[+4.52%, +5.23%]` | improvement, but high-rate/bounded-backpressure wording still applies |
| Stage T tagset cache vs Stage T default | standard v3 UDS | `+0.02%` | `[-0.03%, +0.08%]` | throughput-neutral in this run |
| Stage T tagset cache vs Stage T default | high-rate v3 UDS metrics-only | `+21.17%` | `[+21.01%, +21.32%]` | large parser relief; SMP marks `Regression=true` only because absolute change exceeds its ±20% threshold while `Improvement=true` |
| Stage T tagset cache vs `main` | standard v3 UDS | `+1.60%` | `[+1.43%, +1.77%]` | improvement, lower RSS in this paired run |
| Stage T tagset cache vs `main` | high-rate v3 UDS metrics-only | `+28.42%` | `[+28.21%, +28.64%]` | large parser/backpressure relief; same SMP threshold caveat |

Artifacts:

- [`../stageT_parser_interning_effects.csv`](../stageT_parser_interning_effects.csv)
- [`../stageT_parser_interning_selected_metrics.csv`](../stageT_parser_interning_selected_metrics.csv)
- capture/log directories under `captures/stageT-*`.

## Macro read

- The default SLRU string interner removed whole-cache resets: `main` still reached roughly `21k` resets/worker in the standard run and `42k-44k` resets/worker in high-rate runs; Stage T default/tagset stayed at `0` resets and used bounded individual evictions.
- Stage T default improves throughput vs `main` in the validated v3 UDS paths, but does **not** solve the standard-case RSS gap by itself (`~491 MiB` average RSS vs `~439 MiB` for `main` in the paired standard run).
- The exact tagset cache is the important parser allocation lever for repeated-tagset workloads. In feature-cost runs it reduced per-worker string-interner misses from tens/hundreds of millions to about `47k`, because repeated exact tagsets bypass per-tag interning entirely.
- Standard-case tagset-cache throughput was neutral-to-positive and memory improved materially in paired runs (`~386.8 MiB` average RSS vs `~461.6 MiB` for `main` in the direct standard comparison; `~359.4 MiB` vs `~463.9 MiB` when compared against Stage T default).
- High-rate tagset-cache results are much larger (`+21.17%` vs Stage T default, `+28.42%` vs `main`) and ring telemetry shows the parser bottleneck/backpressure was actually reduced, not merely hidden: in the high-rate feature-cost run max ring lag dropped from a full `~8.39 MiB`/`~2200` records to about `~2.84 MiB`/`~602` records, and blocked time dropped from about `92.7s` on shard 0 to `16.8s`.
- Keep the exact tagset cache opt-in for now despite strong repeated-tagset results. The current SMP cases are exact-tagset-friendly; mostly-unique tagsets still need a feature-cost/admission validation before making it default.
