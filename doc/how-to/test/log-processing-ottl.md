# OTTL log filtering experiment

This prototype adds read-only OTTL conditions to the Agent's log-processing rules.
The experiment measures both the cost of evaluating a condition and the cost of
preparing a log for evaluation. It does not add OTTL masking or transformations.

## Configuration

For integration-level rules, use `log_processing_rules` in the log source.
For global rules, use `logs_config.processing_rules` in `datadog.yaml`. See the
[configuration reference](../../reference/logs/ottl-processing.md) for complete examples.

An integration-level rule list looks like:

```yaml
log_processing_rules:
  - type: exclude_at_match_ottl
    name: drop_successful_health_checks
    pattern: 'attributes["http"]["path"] == "/health" and attributes["http"]["status"] < 400'
  - type: include_at_match_ottl
    name: keep_production
    pattern: 'attributes["env"] == "production"'
```

`exclude_at_match_ottl` drops matching messages. `include_at_match_ottl` drops
nonmatching messages. As in the summer prototype, runtime errors pass the message
through that rule; later rules still execute. Invalid conditions reject the
configuration. Missing JSON fields evaluate to `nil`. Malformed JSON and
out-of-range indexes produce runtime errors. The pass-through behavior means an
include rule is not a strict enforcement boundary on malformed input.

The context exposes:

- `attributes`: the JSON object in the message content. Nested bracket notation,
  array indexes, and computed keys are supported. Duplicate object keys use the
  last value, matching `encoding/json`. Integer values remain `int64`; integers
  outside that range cause an evaluation error when accessed.
- `body`: the original message text, including for non-JSON logs.
- `log.attributes` and `log.body`: aliases for those paths.

The registered converters are `IsMatch`, `HasPrefix`, `HasSuffix`, `IsString`,
`IsInt`, `IsDouble`, `IsBool`, `IsMap`, `IsList`, `Len`, `Int`, `Double`, and
`String`. These are upstream OTTL implementations. Editors and other converters
are deliberately unregistered; this is a filtering subset, not the complete
Collector transform processor or its full log context.

Global rules run before source rules, in their configured order. A legacy
`mask_sequences` rule invalidates the parsed context when it changes the content,
so subsequent OTTL rules see the masked value. Retained messages keep their
original bytes; filtering does not serialize the JSON document.

## Implementation choices

- Compile upstream OTTL conditions during configuration processing.
- Register converters explicitly instead of retaining `StandardFuncs`.
- Evaluate against an Agent context instead of allocating Collector log,
  resource, and scope records for each rule.
- Borrow a context from a concurrency-safe pool for each message that reaches an
  OTTL rule. Parse and validate JSON lazily, once until the message changes.
- Reuse the JSON parser's arena between messages. Convert only fields requested by
  a predicate into OTTL values; object/list converters may materialize subtrees.
- Leave compiled predicates shareable across pipelines; mutable JSON state belongs
  to the borrowed message context.

## Correctness checks

```shell
bazel test --config=dd-agent-go-tests-only \
  //comp/logs/agent/config:config_test \
  //comp/logs-library/processor:processor_test \
  --test_output=errors
```

Use `--config=gorace` to check concurrent use. The tests cover nested and array
access, escaping, duplicate keys, integer precision, invalid input, rule ordering,
source rules, and cache invalidation after masking.

## Microbenchmarks

Wait for builds and other heavy work to finish before timing. Run each target
separately so the two benchmark processes do not compete for CPU:

```shell
bazel test --config=dd-agent-go-tests-only \
  //comp/logs/agent/config:config_test \
  --nocache_test_results --test_output=streamed \
  --test_arg=-test.run=^$ --test_arg=-test.bench=BenchmarkOTTLFiltering \
  --test_arg=-test.benchmem --test_arg=-test.benchtime=200ms \
  --test_arg=-test.count=3 --test_arg=-test.cpu=1

bazel test --config=dd-agent-go-tests-only \
  //comp/logs-library/processor:processor_test \
  --nocache_test_results --test_output=streamed \
  --test_arg=-test.run=^$ --test_arg=-test.bench=BenchmarkOTTLProcessor \
  --test_arg=-test.benchmem --test_arg=-test.benchtime=200ms \
  --test_arg=-test.count=3 --test_arg=-test.cpu=1
```

The comparison includes regex, Ralph's pinned fastjq version, a reconstruction of
his per-rule JSON-to-Collector context, that same context shared between rules,
and the new Agent context. The original context uses the current OTTL version,
so this is a comparison of integration approaches, not a reproduction of the
summer's exact numbers. fastjq is a benchmark-only dependency.

The synthetic payloads are roughly 256 B, 2 KiB, and 16 KiB. Each contains 1, 5,
or 20 evaluations of a nonmatching `level == "debug"` condition; every rule is
reached. Padding precedes the selected field. Compilation is outside the timing.
The processor benchmark additionally covers actual global/source rule dispatch
and context pooling. These timings exclude network, encoding, and intake costs
and cannot establish an end-to-end Agent CPU percentage or a VRL comparison.

## Binary-size comparison

Build `//cmd/agent:agent` at the same base commit, on the same platform, with the
same Bazel flags, once with and once without the implementation. Record exact
byte sizes and whether symbols are stripped. Adding a module requirement does
not imply its whole module is linked, and preexisting dependencies can reduce the
incremental cost. Also compare against a build registering `StandardFuncs` if
measuring the benefit of the restricted function registry.

Before a production decision, repeat the comparison on the Linux release build
and run SMP with realistic matching ratios, varied predicates, mixed legacy
rules, and a no-processing baseline. Measure CPU, allocations, PSS, and
`missed_bytes` below pipeline saturation. Benchmark VRL under the same conditions
before claiming parity with it.

## Rebase onto current main (2026-11-04)

The integration now uses OTTL v1.3.1 and Collector component/pdata v1.70.0,
already selected by `main`, instead of the experiment's OTTL v0.159.0. Rule
evaluation lives in `applyProcessingRules`, shared by the synchronous and
asynchronous processors. The Linux file-tailing fixture includes both upstream
tag filters and the global OTTL condition.

The initial measurements below predate this rebase and dependency update. They
are retained as historical experiment results, not measurements of the current
binary. Repeat timings and binary-size comparisons on the new base before using
them for a release decision.

## Initial measurements (2026-10-29)

Both Agent builds use base commit `e790a15653edee5ea25f864c7e56cd3e075655a3`
on macOS/arm64 and the default `bazel build //cmd/agent:agent` settings. The
registry comparison keeps the Agent JSON context unchanged and changes only the
function registration. These are executable file sizes, excluding runfiles:

| Build | Bytes | Increase over baseline |
| --- | ---: | ---: |
| Unchanged Agent | 107,329,426 | — |
| OTTL with filtering converters | 109,899,858 | 2.45 MiB (+2.39%) |
| OTTL with `StandardFuncs` | 112,090,706 | 4.54 MiB (+4.44%) |

Registering filtering converters instead of `StandardFuncs` saves **2.09 MiB**
in this controlled comparison. The resulting executable has no DWARF debug
sections. These sizes do not establish the Linux release/package delta and are
not directly comparable to the summer's different Agent/OTTL versions.

Raw build logs and benchmark output are saved under
`bin/benchmarks/ottl-filtering/` in the experiment checkout (ignored by Git).

### Filtering timings

Apple M5 Pro, macOS/arm64, one benchmark CPU, median of three 200 ms runs.
Values are **microseconds per message**, excluding compilation. `fastjq` below
uses its reusable-buffer API. The original OTTL column reconstructs Ralph's
per-rule JSON-to-Collector context using the current OTTL version.

| JSON bytes | Rules | fastjq | Agent OTTL | Original OTTL integration |
| ---: | ---: | ---: | ---: | ---: |
| 250 | 1 | 0.104 | 0.270 | 1.750 |
| 250 | 5 | 0.516 | 0.452 | 8.641 |
| 250 | 20 | 2.047 | 1.113 | 35.672 |
| 2,042 | 1 | 0.135 | 0.729 | 6.793 |
| 2,042 | 5 | 0.699 | 0.942 | 33.730 |
| 2,042 | 20 | 2.720 | 1.601 | 128.427 |
| 16,378 | 1 | 0.430 | 4.669 | 45.694 |
| 16,378 | 5 | 2.167 | 4.898 | 220.967 |
| 16,378 | 20 | 8.696 | 5.521 | 914.652 |

For the 2,042-byte payload, five OTTL rules are about 35% slower than fastjq,
while twenty are about 41% faster. A single rule remains about 5.4 times slower.
For the 16,378-byte payload, a single rule is about 10.9 times slower. These
results support shared parsing as a useful integration optimization, not an
intrinsic language ranking: fastjq runs each configured rule separately here,
and combining its predicates could change the comparison.

At five rules on the 2,042-byte payload, the Agent context allocates 100 B and
10 objects per message, versus 17,400 B and 180 objects for the reconstructed
original integration. This is allocation churn, not resident-memory measurement.

### Remaining input-preparation cost

`BenchmarkOTTLValidation` isolates the long-string case. At roughly 16 KiB,
arena parsing alone takes about 0.32 microseconds, while `fastjson.ValidateBytes`
takes about 4.18 microseconds. Strict validation dominates this workload. The
already-pinned experimental JSON library's `jsontext.Value.IsValid` took about
4.74 microseconds and allocated once; `encoding/json.Valid` took about
24.73 microseconds. Neither replaced the existing validator.

These validation results use well-formed inputs. The validators differ in some
Unicode/duplicate-key policies, so the benchmark is not a conformance claim.
The experimental JSON import is confined to benchmarks.

### Processor dispatch and cleanup

The actual processor benchmark uses a 68-byte log and splits its rules between
one global rule and the remaining source rules. Median timings in nanoseconds
per message after moving context cleanup out of the rule loop:

| Rules | Regex | OTTL | OTTL bytes / allocations per message |
| ---: | ---: | ---: | ---: |
| 1 | 123.2 | 201.3 | 20 B / 2 |
| 5 | 584.8 | 441.9 | 100 B / 10 |
| 20 | 2330.0 | 1110.0 | 400 B / 40 |

Cleanup no longer allocates a deferred call per message. Regex-only processing
still allocates zero objects in this benchmark. Full Agent builds, both unit-test
suites, race-enabled runs, and build-file formatting checks passed. Linux release
size, SMP load behavior, and an equivalent current VRL implementation remain
unmeasured.
