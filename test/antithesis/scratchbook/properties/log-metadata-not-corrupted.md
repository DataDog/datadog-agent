---
slug: log-metadata-not-corrupted
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# log-metadata-not-corrupted — Log Metadata Is Not Corrupted at Delivery

## What Led to This Property

The existing property catalog is loss-biased: most properties ask whether a
message is delivered, not whether it is delivered *with the correct metadata*.
Metadata corruption is a higher-probability failure mode in production than
outright loss: it silently mislabels logs, corrupts service-level dashboards,
and may route MRF payloads to wrong destinations.

Two concrete corruption paths were identified during investigation:

1. **`processor.go` MRF tagging uses `msg.Origin.Service()`**: `filterMRFMessages`
   in the processor reads `msg.Origin.Service()` to check whether the message
   belongs to the MRF service allowlist. `Origin.Service()` prefers
   `LogSource.Config.Service` (static config) over the per-message `o.service`
   field. Under container churn (rapid container create/destroy cycling), the
   `LogSource` backing a message can be reassigned or recycled, leaving
   `Config.Service` stale. A message from container B may be tagged for MRF
   routing based on container A's service name.

2. **Kubernetes parser `status` on header-parse failure**: In
   `kubernetes.go:46-88`, if the Kubernetes log line has fewer than 3
   space-separated components, `parseKubernetes` returns with `status =
   message.StatusInfo` (the default, even if the line content is stderr).
   If timestamp parsing fails, the error is returned but `msg.Status` is already
   set from the stream field. These are correctness-bounded failures, but a
   header parse failure also means `msg.ParsingExtra.Timestamp` is empty, which
   makes the downstream timestamp resolution fall back to `time.Now()` — a
   potential metadata corruption for log ordering.

3. **`adaptive_sampler_sampled_count` tag from a prior state**: The sampler
   appends `adaptive_sampler_sampled_count:<n>` to `msg.ParsingExtra.Tags`
   (sampler.go:222). `ParsingExtra.Tags` is accumulated during processing and
   not reset between message passes. If a message object is reused (e.g., from
   a pool or via a bug in the decoder pipeline), stale sampled_count tags from
   a prior message can leak onto subsequent messages.

## Code Paths Involved

**`comp/logs-library/processor/processor.go:225-240`** — `filterMRFMessages()`:

```go
func (p *Processor) filterMRFMessages(msg *message.Message) {
    serviceAllowlist := p.failoverConfig.failoverServiceAllowlist
    if len(serviceAllowlist) == 0 {
        msg.IsMRFAllow = true
        return
    }
    _, serviceMatch := serviceAllowlist[msg.Origin.Service()]
    if serviceMatch {
        msg.IsMRFAllow = true
    }
}
```

`msg.Origin.Service()` in `pkg/logs/message/origin.go:154-161`:

```go
func (o *Origin) Service() string {
    if o == nil || o.LogSource == nil {
        return ""
    }
    if o.LogSource.Config.Service != "" {
        return o.LogSource.Config.Service  // static config wins
    }
    return o.service
}
```

The static config value is set from the AD annotation or integration config at
source creation time. Under container churn, a recycled LogSource may have a
stale `Config.Service` until AD reschedules and creates a new source.

**`comp/logs-library/processor/json.go:62-70`** — `jsonEncoder.Encode()`:

```go
encoded, err := json.Marshal(jsonPayload{
    ...
    Service:   msg.Origin.Service(),    // stale if origin recycled
    Source:    msg.Origin.Source(),     // same risk
    Tags:      msg.TagsToString(),      // includes adaptive_sampler_sampled_count
    Hostname:  hostname,
})
```

This is the final wire format written to the payload. Any corruption of
`Service`, `Source`, or `Tags` at this point is what is delivered to intake.

**`pkg/logs/internal/parsers/kubernetes/kubernetes.go:46-88`** — `parseKubernetes()`:

```go
if len(components) < 3 {
    return message.NewMessage(msg.GetContent(), nil, status, 0), errors.New("cannot parse the log line")
}
// status from components[1] (stdout→info, stderr→error)
status = getStatus(components[1])
...
timestamp = string(components[0])
_, err := time.Parse(time.RFC3339Nano, timestamp)
if err != nil {
    return msg, errors.New("invalid timestamp format")
    // msg.Status is already set from components[1], but ParsingExtra.Timestamp is empty
}
```

On timestamp parse failure, `msg.Status` is set but `msg.ParsingExtra.Timestamp`
is empty. The downstream encoder calls `time.Now().UTC()` for the timestamp,
which is the collection time rather than the original log time. This is a
timestamp metadata corruption for events that arrived with a corrupted timestamp.

**`pkg/logs/internal/decoder/preprocessor/sampler.go:222`** — tag append:

```go
msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, adaptiveSamplerSampledCountTag(e.sampled))
```

This appends to the existing slice. If `ParsingExtra.Tags` is not reset between
messages (not shown to be an issue currently, but a risk if message reuse is
introduced), stale tags would appear.

## What Goes Wrong

- **Wrong MRF routing**: a log from container B with service "payment-service" is
  tagged `IsMRFAllow = true` because the stale LogSource says
  `Config.Service = "billing-service"` (the MRF allowlisted service). The log
  goes to the secondary region. Or the inverse: a billing-service log is not MRF-
  tagged because the origin says service is "payment-service" (not allowlisted).

- **Wrong `ddsource`/`service` at intake**: an operator's dashboard shows logs
  attributed to the wrong service. Alerting based on `service:billing-service`
  misses events that were mislabeled as `payment-service`.

- **Wrong `status` on K8s stderr lines**: a stderr line where the header fails to
  parse gets `status: info` instead of `status: error`. Severity-based alerting
  misses errors.

## Assertion Design

**`Always`** (workload-side): The workload writes lines to known log files where
the expected metadata is known:
- File X is associated with `service = "test-service-a"`, `ddsource = "test-src-a"`.
- Every line received at fakeintake from source X has `service == "test-service-a"`
  AND `ddsource == "test-src-a"` AND `hostname == <expected-host>`.

Assert that no line has a different service, source, or hostname than expected.
Under container churn (Antithesis starts/stops containers rapidly), this catches
MRF tagging and origin contamination.

**`Always`** (workload-side, K8s status): For lines written to stderr in a K8s
container context, every received message has `status == "error"`. A header-parse
failure causing `status == "info"` is caught.

**`AlwaysOrUnreachable`** (SUT-side, sampled_count stale tag): After any message
is written to `outputChan` in the processor, assert that `msg.ParsingExtra.Tags`
contains at most one `adaptive_sampler_sampled_count:` entry. Multiple entries
indicate stale tag accumulation.

## Why It Matters

Metadata corruption is harder to detect than loss. Missing logs create gaps in
dashboards; mislabeled logs fill dashboards with wrong data. Operators may
investigate the wrong service, missing the real source of errors. In production:
- Container churn at Kubernetes scale can expose the origin-recycling hazard.
- High-volume stderr-heavy workloads (Java exception stacks) stress the K8s
  parser header detection.
- MRF deployments that rely on service-based routing are directly affected by
  the MRF tagging hazard.

## Relationship to Other Properties

- `secrets-redacted-before-send` — covers content integrity (redaction); this
  property covers metadata integrity.
- `logs-not-modified-in-transit` — covers the pipeline's no-modification
  guarantee for log content; this property covers the envelope metadata fields.
- `container-identifier-no-collision` — covers tailer ID collision; if identifiers
  collide, the origin contamination hazard becomes more likely.

## Open Questions

- Under what container churn rate does `LogSource.Config.Service` become stale
  before the message is processed? This determines how easily the MRF tagging
  hazard is reachable. `(partial: stale origin is most likely during the window
  between container stop and AD scheduler rescheduling a new source; the window
  is bounded by the AD scan interval, typically ~5s)`
- Does `ParsingExtra.Tags` get reset to `nil` for each new message, or is it
  accumulated across messages from the same tailer? `(partial: message objects
  are created fresh in tailer tail() loops, so accumulation across messages is
  not currently a risk; the stale-tag scenario requires message object reuse)`
- Is there a fakeintake endpoint that exposes per-log-line metadata fields
  (service, ddsource, ddtags) for workload-side assertion? `(needs human input)`

### Investigation Log

#### Does msg.Origin.Service() read stale data under container churn?

- Examined: `comp/logs-library/processor/processor.go:225-240`, `pkg/logs/message/origin.go:154-161`, `sut-analysis.md §10` (container churn / container_collect_all startup race).
- Found: `Origin.Service()` returns `LogSource.Config.Service` if non-empty. LogSource is created by the AD scheduler at container discovery time. During container churn (rapid start/stop), the old LogSource may persist in the message's origin for the duration of the pipeline (up to several seconds). The `Config.Service` value is set from the AD annotation or integration config and is not updated once set. A recycled container with a different service annotation will get a new LogSource, but in-flight messages from the old container still reference the old LogSource.
- Not found: any mechanism that clears `Config.Service` on container stop or that validates freshness of the origin at encode time.
- Conclusion: the hazard is real but probabilistic. It requires a message to be in-flight (in the pipeline channel) when its container stops and a new container starts with a different service annotation.

#### Does the Kubernetes parser produce a wrong status on header-parse failure?

- Examined: `pkg/logs/internal/parsers/kubernetes/kubernetes.go:46-88`, `pkg/logs/internal/parsers/kubernetes/kubernetes_test.go`, `pkg/logs/internal/parsers/kubernetes/kubernetes_fuzz_test.go`.
- Found: On `len(components) < 3`, the function returns a new Message with `status = message.StatusInfo` and an error. On timestamp parse failure, `msg.Status` is already set to the parsed stream status, but `ParsingExtra.Timestamp` is empty. The fuzz test confirms various malformed inputs are handled without panic, but it does not assert that `status` is correct for all parse-failure cases.
- Not found: a test that specifically asserts `status == "error"` for a malformed stderr line that fails timestamp parsing.
- Conclusion: the status-corruption risk is bounded to the `len(components) < 3` case only (returns `StatusInfo` regardless of content). The timestamp-failure case sets status correctly from the stream field.
