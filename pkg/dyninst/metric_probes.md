# Metric probes for Go Dynamic Instrumentation

Status: implemented (2026-10-11).

This document describes how dyninst (Go Dynamic Instrumentation, running in
system-probe) implements `METRIC_PROBE`s: probes that turn calls to a function
or hits on a source line into a DogStatsD count or gauge.

## Background

### Metrics in Datadog

A metric is a series of timestamped values with a name and a set of tags
(`key:value` strings). Each combination of a name and tag values is a separate
time series, which the Agent calls a context. Queries select contexts by tag
and aggregate them:
`sum:dynamic.instrumentation.metric.probe.orders{env:prod} by {service}`
adds up the matching contexts and draws one line per service. Datadog bills
custom metrics by the number of distinct combinations of name and tag values,
`host` included, so a tag with many values is expensive.

Applications usually don't send metrics to Datadog directly. A DogStatsD
client library, such as datadog-go, sends text datagrams to the Agent's
DogStatsD server over UDP (port 8125) or a Unix socket:

```
dynamic.instrumentation.metric.probe.orders:3|c|#team:checkout,env:prod
```

The datagram holds the name, the value, the type (`c` for count) and the tags.
A client can also send only a fraction of the values and say so with a sample
rate (`|@0.1`), which DogStatsD uses to scale counts back up. DogStatsD
aggregates what it receives per context into 10-second buckets, and the Agent
sends the finished buckets to Datadog every 15 seconds. Clients aggregate too:
datadog-go combines each context's values for 2 seconds before it sends them,
so that it sends fewer datagrams.

The type decides how DogStatsD aggregates a bucket:

| Type | What DogStatsD sends for each 10s bucket | Stored as |
|---|---|---|
| Count (`c`) | the sum of the values divided by 10, a rate per second | RATE; `.as_count()` in a query turns it back into counts |
| Gauge (`g`) | the last value | GAUGE |
| Histogram (`h`) | by default, the max, median, average, count and 95th percentile of the values, as five metrics (`<name>.max`, …) | GAUGE; RATE for `<name>.count` |
| Distribution (`d`) | a sketch of the values, which the backend merges with the other hosts' sketches | DISTRIBUTION |
| Set (`s`) | the number of distinct values | GAUGE |

Each Agent computes a histogram's percentiles from its own values, so they
describe one host. A distribution's percentiles are computed by the backend
from the values of every host.

Tags come from three places:

- The code that sends the metric.
- The client library. datadog-go adds `env`, `service` and `version` from
  `DD_ENV`, `DD_SERVICE` and `DD_VERSION` in the sending process's
  environment, and `dd.internal.entity_id` from `DD_ENTITY_ID`, which the
  Cluster Agent's admission controller sets to the pod's UID.
- The Agent. It adds `host` and, through origin detection, the tags of the
  container or pod that sent the metric. It identifies that origin from the
  Unix socket's peer credentials, from a container ID that the client puts in
  the datagram (`|c:<id>`), or from a `dd.internal.entity_id:<pod UID>` tag,
  which it then removes. The first two are off by default in the Agent's
  configuration; the tag needs no setting. `dogstatsd_tag_cardinality` decides
  how many of the origin's tags are added: `low`, the default, adds tags such
  as `kube_namespace` and `kube_deployment`; `orchestrator` also adds
  `pod_name`; `high` also adds `container_id`.

For metric probes, this means:

- DogStatsD adds up the counts it receives, so the reporter sends the change
  since its previous send, never the running total.
- A count probe's metric is stored as a per-second rate, like any DogStatsD
  count; queries use `.as_count()` to get counts.
- A probe's tags are fixed strings from its definition, so the number of
  contexts a probe creates depends on where its service runs (hosts, and pods
  at `orchestrator` cardinality), not on the values it sees.

### What other tracers do

A metric probe is configured through Remote Config like any other probe:

```json
{"id": "…", "type": "METRIC_PROBE", "language": "go", "version": 1,
 "where": {"methodName": "main.handle"}, "evaluateAt": "EXIT",
 "kind": "COUNT", "metricName": "orders",
 "value": {"dsl": "len(items)", "json": {"len": {"ref": "items"}}},
 "tags": ["team:checkout"]}
```

| Tracer | Count | Gauge | Histogram | Distribution | Probe ID tag |
|---|---|---|---|---|---|
| Java | ✅ (integers only) | ✅ | ✅ | ✅ | `debugger.probeid` |
| Python | ✅ | ✅ | ✅ | ✅ | `debugger.probeid` |
| .NET | ✅ | ✅ | ✅ | ❌ | `probe-id` |
| PHP | ✅ | ✅ | ✅ | ✅ | none |
| Node.js, Ruby | ❌ | ❌ | ❌ | ❌ | |

Every tracer sends `dynamic.instrumentation.metric.probe.<metricName>` through
its DogStatsD client, with sample rate 1 and no rate limiting. The value is an
expression in the DI expression language (paths, `len`, literals; no
arithmetic). A Count without a value counts calls. Java checks the value's
type when the probe is installed; at run time, a value that can't be evaluated
(for example a nil pointer along the path) skips the call.

The backend's "last captured" query (debugger-backend
`MetricProbeDeserializer.kt`) relies on the metric name prefix. The UI only
offers metric probes for a service if the backend's version matrix says the
language supports them; for Go it says "not supported"
(dd-source `domains/debugger/apps/apis/debugger-edge-api/version_matrix.go`).

### Go before this change

`rcjson` parsed `METRIC_PROBE`s into `rcjson.MetricProbe`, and
`irgen.makeInterests` rejected them with an `UnsupportedFeature` issue, so the
user saw an ERROR status.

## Goals and non-goals

Goals:

- Count and Gauge metric probes on function entry and on source lines, with
  optional `when` conditions.
- Cheap enough to put on hot functions: no per-call ringbuf events, no stack
  walks, no userspace work per call.
- Metrics that look like a tracer's: same name, same probe ID tag,
  service/env/version, and Kubernetes pod tags.

Non-goals for this version:

- Histogram and Distribution. They are rejected with a clear error. The design
  leaves room for them (see [Later: histograms and
  distributions](#later-histograms-and-distributions)).
- Evaluating at function exit: `evaluateAt` is ignored, as it is for Go log
  probes today, and values that use `@return` or `@duration` are rejected.
- Container tags for targets outside Kubernetes.
- Any change to dd-trace-go. Its Remote Config handler already accepts every
  probe type, and dyninst already gets service/env/version and the container
  ID from it.

## Behavior

### Kinds and values

| | No `value` | Allowed value types | Semantics |
|---|---|---|---|
| Count | adds 1 per call | signed and unsigned integers, `bool` (true counts as 1) | the Agent sums the values in each 10s bucket |
| Gauge | rejected | integers, floats, `bool` | the most recent value is sent |
| Histogram, Distribution | rejected (`UnsupportedFeature`) | | |

- A value of any other type (string, struct, slice, …) makes the probe fail
  installation with an ERROR status naming the expression and its type.
- Counts accumulate as `int64`. An unsigned value above `math.MaxInt64` wraps.
- Kind names are matched case-insensitively (`COUNT`, `count`).

### When the probe fires

- Function probes fire at function entry. `evaluateAt` is ignored. No return
  probe is attached, so metric probes don't use `in_progress_calls`.
- Line probes fire when the line is reached.
- A `when` condition is evaluated first. If it is false, the call is not
  counted. If it can't be evaluated, the call is not counted and is reported
  as an evaluation error.
- There is no rate limiting. The per-probe CPU circuit breaker still applies.

### What gets sent

Every couple of seconds, for each probe that was updated since the previous
send:

- Count: `Count(name, delta)` where `delta` is the change in the running sum.
- Gauge: `Gauge(name, value)` with the most recent value.

| Tag | Source |
|---|---|
| the probe's own `tags` | probe definition |
| `debugger.probeid:<id>` | probe definition |
| `service`, `env`, `version` | the target's tracer, as reported to dyninst |
| `dd.internal.entity_id:<pod UID>` | workloadmeta, for Kubernetes targets. DogStatsD removes this tag and adds the pod's tags at the core agent's `dogstatsd_tag_cardinality` |
| `host` | added by the core agent |

Metric names and tags come from users, so characters that would break a
DogStatsD datagram (`|`, `,`, `#`, `:` in names, newlines) are replaced with
`_`.

Metrics go through a dedicated datadog-go client created with
`WithoutOriginDetection()`, so DogStatsD never attributes them to
system-probe's own container. datadog-go also adds `DD_ENV`, `DD_SERVICE` and
`DD_VERSION` from system-probe's environment as tags; if a deployment sets
those on system-probe (for example through Helm's `datadog.env`), metric probe
metrics carry a second `env`/`service`/`version` value. We accept this.

### Probe status

- RECEIVED and INSTALLED as for other probes.
- EMITTING the first time a send finds a probe updated.
- ERROR at install time for: unsupported kind, missing or invalid value, value
  of an unsupported type, `@return`/`@duration` in the value, and the usual
  location problems.

### Evaluation errors

When the value or the condition can't be evaluated (nil pointer, index out of
range, variable not available at that PC), the call is skipped and the kernel
counts the failure. When a send finds new failures for a probe, dyninst sends
an evaluation-error event through the regular snapshot uploader, at most once
per probe every 5 minutes. The event names the expression and the error, so it
shows up with the probe in the UI.

## Design

```
RC ──▶ rcjson.MetricProbe ──▶ irgen ──▶ compiler ──▶ BPF program
                              (entry-only event,    (cond → value →
                               value expression)     METRIC_UPDATE)
                                                          │ per call
                                                          ▼
                                             metrics_buf[probe_id] (ARRAY map)
                                                          │ every 2s
                                                          ▼
                              metrics reporter (module) ──▶ DogStatsD (core agent)
```

### rcjson

`MetricProbe` gains `When` and implements a new optional interface:

```go
// MetricProbeDefinition is implemented by probe definitions of kind
// ProbeKindMetric.
type MetricProbeDefinition interface {
	ProbeDefinition
	GetMetricKind() MetricKind // Count, Gauge, Histogram, Distribution
	GetMetricName() string
	// GetValue returns the value expression, or nil if there is none.
	GetValue() (dsl string, json json.RawMessage)
}
```

An optional interface, rather than new methods on `ir.ProbeDefinition`, keeps
the many other implementations unchanged.

### IR generation

- `rcjson.Validate`, which the module runs on every probe it receives, rejects
  metric probes without a name or with an unknown kind. irgen checks what Go
  supports.
- `makeInterests` accepts Count and Gauge metric probes and keeps rejecting the
  other kinds.
- `newProbeInstance` skips return events for metric probes, as
  `WithSkipReturnEvents` does globally. Injection points get
  `NoReturnReasonReturnsDisabled`.
- `analyzeAllProbes` treats the value like a capture expression bound to the
  entry or line event, under a new `RootExpressionKindMetricValue`. Install-time
  issues are recorded on the analyzed probe and reported by
  `populateInstanceExpressions`, as condition issues are.
- After resolution, the value's Go kind is checked against the table above.
- The runtime.recovery probe is only needed for probes with return events, so
  it is not added for programs whose function probes are all metric probes.

### Compiler and BPF

**Event handler.** A metric event's handler is:

```
[CALL condition] → METRIC_COND_ERROR_CHECK → PREPARE_EVENT_ROOT → [CALL value] → METRIC_UPDATE → RETURN
```

- `METRIC_COND_ERROR_CHECK {probe_id}` is needed because each expression starts
  with `EXPR_PREPARE`, which clears `condition_eval_error`. If the condition
  failed to evaluate, this op counts an error and aborts.
- `METRIC_UPDATE {probe_id, kind, value_kind, value_size, value_offset}` reads
  expression status 0 and the value from the event root data in the scratch
  buffer. If the status isn't PRESENT, it counts an error. Otherwise it widens
  the value to 64 bits (sign- or zero-extended for integers, raw bits for
  floats) and updates the map:
  - Count: atomically adds the value, or 1 when there is no value.
  - Gauge: stores the bits.

  Either way it then increments `updates`.

**Map.** Per-probe state lives in a new `metrics_buf`
`BPF_MAP_TYPE_ARRAY` keyed by probe ID. It is sized like `stats_buf` and
updated with `__sync` atomics, so it is safe across CPUs:

```c
typedef struct metric_state {
  int64_t sum;           // Count: running sum.
  uint64_t value;        // Gauge: bits of the most recent value.
  uint64_t updates;      // Number of successful updates.
  uint64_t eval_errors;  // Number of calls skipped because of evaluation errors.
  uint8_t last_error;    // Kind of the most recent evaluation error.
  uint8_t __padding[7];
} metric_state_t;
```

**Fast path.** `probe_params_t` gets an `is_metric` byte, taken from its
padding. For metric injection points, `probe_run` skips reading the goroutine
ID, the stack walk and hash, `in_progress_calls`, pointer chasing and the
ringbuf submit. It initializes the scratch buffer, runs the stack machine and
returns. The compiler gives metric events `ThrottleNone`, because their budget
is infinite anyway.

**Loader.** The loader sizes `metrics_buf` and exposes `Program.MetricStates()`,
which mirrors `Program.RuntimeStats()`. `types_linux.go` is regenerated with
`bazel run //pkg/dyninst/loader:types_godefs`.

### Userspace reporting

A module-level `metricsReporter` with a single goroutine:

- **Registration.** `runtimeImpl.Load` registers programs that contain metric
  probes. It passes the loaded program, each probe's kind, name, value type and
  tags, the process identity (service/env/version, container ID), and a logs
  uploader for evaluation-error events.
- **Each tick** (2s), for each registered program:
  - read `MetricStates()` and diff against the previous read;
  - send counts and gauges;
  - mark probes EMITTING;
  - send rate-limited evaluation-error events.
- **Final report.** When a program is detached, the runtime reports its metric
  probes one last time: after the uprobes are removed, so no hit is missed, and
  before the program is unlinked from its process, so probes first hit right
  before a process exits still become EMITTING. Without it, a recompile (adding
  or removing any probe on the process) or a process exit would lose up to one
  tick of data. `loadedProgramImpl.Close` then unregisters the program before
  the BPF maps are closed.
- **Pod UID** is looked up in workloadmeta when tags are built. system-probe
  already streams containers and pods from the core agent; a container's
  `Owner` is its pod. If the container isn't known yet, the lookup is retried on
  later ticks.
- **Statsd client.** The `cmd/system-probe` module factory creates the
  dedicated client with the statsd component's `CreateForHostPort` (same
  address and options as system-probe's shared client, plus
  `WithoutOriginDetection()`) and passes it to the module along with
  workloadmeta.

### Reloads

A process has one BPF program that holds all of its probes, and the program is
rebuilt whenever the process's probes change: a probe is added, removed or
updated to a new version. A circuit-breaker trip (below) and the discovery of
new types also rebuild it. A rebuild replaces the old program with a new one,
and each program has its own `metrics_buf`, so the metric state of every metric
probe on the process starts over, not only that of the probes that changed:

- The old program is detached first, and the reporter reports it one last time
  after its uprobes are gone, so every hit up to the detach is sent. Pending
  evaluation errors are sent too, unless one was sent less than 5 minutes
  before.
- Hits between the detach and the attach of the new program, the time it takes
  to generate, compile and load it, are not counted. This applies to every
  probe on the process.
- The reporter registers the new program as a new one: its counts start from
  zero, and only increments are sent, so nothing is counted twice and counts
  show no reset in Datadog. A gauge keeps its last value until the new program
  sees a hit.
- A probe that changed is reported under its new definition (metric name, kind
  and tags) from the new program on; the old program's final report used the
  old one.
- Probe statuses are tracked per probe version, so probes that didn't change
  stay EMITTING. Evaluation-error rate limiting starts over with the new
  program, so a probe whose evaluation keeps failing can send one extra
  evaluation-error event per rebuild.

Process restarts work the same way: the old process's program is detached when
the process exits, and the new process gets a program of its own.

### Overhead and the circuit breaker

Metric probes have no rate limit, so the per-probe CPU circuit breaker is what
keeps a probe on a hot function from slowing the target down. It treats metric
probes like any other probe:

- Every `circuit_breaker.interval` (1s by default), the actuator reads each
  probe's hit count and BPF CPU time from `stats_buf`, and charges
  `interrupt_overhead` (2µs by default) per hit for the uprobe trap. A probe
  that uses more than `per_probe_cpu_limit` (0.1 CPU by default) trips. If all
  probes together use more than `all_probes_cpu_limit` (0.5 CPU), the most
  expensive one trips.
- A metric probe's BPF time covers its condition, its value and
  `METRIC_UPDATE`, but no stack walk and no ringbuf submission, so a hit costs
  less than a log probe's. The interrupt charge alone caps a probe at 50,000
  hits per second, so on hot functions metric probes trip at about the same
  rate as log probes, which pay for the interrupt even when throttled.
- A trip reports an ERROR with the CPU usage, adds the probe to the process's
  circuit-broken set (until the probe's version changes), and recompiles the
  process's program without it. The recompile detaches the old program first,
  which reports the probe's last counts, and then the metric stops.
- The rebuild affects the other metric probes on the process as described in
  [Reloads](#reloads).

### Later: histograms and distributions

These fit the same structure:

- a map of buckets with an entry per probe, like `metrics_buf`, updated by
  `METRIC_UPDATE` with integer log-linear bucketing (the exponent and mantissa
  bits for floats) plus exact min, max, sum and count;
- a reporter branch that sends weighted samples;
- support for `evaluateAt: EXIT` and `@duration`, which needs entry/return
  pairing and a return-side `METRIC_UPDATE`.

## Alternatives considered

- **One ringbuf event per call, aggregated in userspace.** Simpler, but every
  call costs a ringbuf event plus decoding in system-probe. Metric probes are
  unthrottled, so hot functions would trip the circuit breaker.
- **Looking up container tags in system-probe with the tagger.** Gives
  per-container tags everywhere, but needs `dogstatsd_tag_cardinality` shared
  with system-probe through config sync. The pod UID route needs no Agent
  change and lets DogStatsD apply its own setting.
- **Writing DogStatsD datagrams directly** to avoid datadog-go's
  environment-derived tags. We chose to keep using datadog-go.

## Changes outside this repository

- **dd-source:** add a Go `ProbeTypeMetric` entry with line and method support,
  gated on the Agent version that ships this, in
  `debugger-edge-api/version_matrix.go`. This is what makes the UI offer metric
  probes for Go services.
- **system-tests:** enable `test_metric_status`, `test_metric_line_status` and
  `test_metric_method_expression` for golang in `manifests/golang.yml`.
- **debugger-backend (existing gap, all languages):** the "last captured" query
  doesn't filter by `debugger.probeid`.

## Commit plan

Each commit builds and passes tests on its own. Metric probes stay rejected in
production until the last commit; earlier commits are exercised through an
irgen option that only tests set. Refactors that make the feature commits
smaller come first, in commits of their own that don't change behavior.

1. **dyninst: describe the design of metric probes.** This document.
2. **rcjson: add nil-safe accessors to When.** Refactor.
3. **rcjson: parse metric probe definitions.** Kind, name, value and `when`;
   `ir.MetricKind` and `ir.MetricProbeDefinition`; validation (name required,
   known kind).
4. **irgen: turn variable availability closures into functions.** Refactor, so
   that the metric code can reuse them.
5. **irgen: generate IR for count and gauge metric probes.** Behind
   `irgen.WithMetricProbes(true)`: entry-only events, the value expression with
   its type checks and install-time issues, conditions, and no runtime.recovery
   probe for metric-only programs.
6. **ebpf: move goroutine and stack capture out of probe_run.** Refactor, so
   that metric probes can skip them.
7. **dyninst: aggregate metric probe values in the kernel.** `metric_state_t`
   and `metrics_buf`, `METRIC_COND_ERROR_CHECK` and `METRIC_UPDATE`, the
   `is_metric` fast path in `probe_run`, compiler emission and `ThrottleNone`,
   and `Program.MetricStates()`.
8. **module: report metric probe values to DogStatsD.** The metrics reporter,
   tags, EMITTING, the final report on detach, and the dedicated statsd client
   in the system-probe factory.
9. **module: tag metric probe metrics with the target's pod.** workloadmeta
   wiring and `dd.internal.entity_id`, plus a `TODO(andrei)` that explains that
   only Kubernetes pod-level tags are supported and the options for later.
10. **decode: extract the probe location into a function.** Refactor.
11. **module: extract the logs uploader metadata into a function.** Refactor.
12. **module: report metric probe evaluation errors.** Rate-limited
    evaluation-error events through the logs uploader.
13. **dyninst: enable count and gauge metric probes.** The module turns the
    irgen option on, and an integration test runs metric probes on the `simple`
    test program.
14. **dyninst: rename waitForDropTesterPort to waitForListeningPort.**
    Refactor, so that the next test can use it.
15. **dyninst: test that metric probe counts grow while the target runs.**

## Testing

Each layer has unit tests: rcjson parsing and validation, irgen issues, the
compiler's encoding and handlers, the loader's struct layout, and the reporter
(deltas, tags, the pod tag, evaluation errors). The integration test,
`TestMetricProbes`, puts metric probes on functions of the `simple` test
program, which calls each of them twice with known arguments:

| Probe | Expected |
|---|---|
| Count on `main.condInt64`, no value | 2 |
| Count on `main.condInt64`, value `x` (42, 7) | 49 |
| Count on `main.condInt64`, value `x`, `when tag == "match"` | 42 |
| Count on `main.condUint64`, value `x` | 49 |
| Count on `main.condBool`, value `x` (true, false) | 1 |
| Gauge on `main.condPtrStructArg`, value `x.F64` | the last call's value |
| Count on `main.condNilPtrStruct`, value `x.I32` (&{300}, nil) | 300, with one evaluation error |
| Count line probe in `main.condLine`, value `n` (7, 0) | 7 |

The gauge reads a struct field rather than a float argument because float
arguments are passed in floating-point registers, which the BPF program can't
read. The fake DogStatsD client also checks names and tags, and the test checks
probe statuses and the evaluation-error event.

The `simple` program makes all its calls at once and exits, so those counts
could all come from the final report at detach. `TestMetricProbeCountGrows`
covers the periodic reports: it puts a count probe on
`main.LookAtTheRequest` in `rc_tester`, an HTTP server that keeps running,
sends 3 requests and then 2 more, and checks that later reports bring the count
from 3 to 5 with only positive increments.

dyninst needs Linux 5.17 or later (`bpf_loop`). CI runs the dyninst tests in the
KMT jobs' `no_usm` test set, on about 20 distributions for each of x86_64 and
arm64 (`.gitlab/test/kernel_matrix_testing/system_probe.yml`); on older kernels
the tests skip themselves. That covers kernels that attach one uprobe per
address and kernels at 6.10 or later, which use `uprobe_multi`.

## Known gaps

- Type redaction: metric values are checked against redacted identifiers, but
  not against `DD_DYNAMIC_INSTRUMENTATION_REDACTED_TYPES`, which only the
  decoder applies. See the `TODO(andrei)` on `checkMetricValueType`.
- EMITTING after ERROR. A metric probe becomes EMITTING when a report first
  finds it updated, up to 2s after its first hit, while the circuit breaker
  checks every second. A probe that is hot from the start can trip, and get its
  ERROR, before its first report; that report (the old program keeps running
  until the recompile replaces it) then sends EMITTING, which the diagnostics
  manager doesn't suppress for probes that already have an ERROR. The probe's
  last status is then EMITTING, which hides the error. Not reporting EMITTING
  for a probe version that has an ERROR would fix it, for log probes too.
- Container tags outside Kubernetes, and per-container tags. See the
  `TODO(andrei)` on `resolveContainerLocked`.

## Open questions

- Is a 2s send interval right? A shorter interval makes EMITTING and new data
  appear sooner, at the cost of more datagrams. The Agent buckets by 10s
  either way.
- Which Agent version should gate the backend's version matrix entry?
