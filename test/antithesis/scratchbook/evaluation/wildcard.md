---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
external_references:
  - path: https://datadoghq.atlassian.net/wiki/spaces/~602449d8f3d296006864db68/pages/6495210537/Property+testing+Logs+Agent+Adaptive+Sampling
    why: Owner's design doc; defines sampling correctness + read-liveness properties.
  - path: https://datadoghq.atlassian.net/wiki/spaces/~712020006700eab4c247639d448c47103cd8b7/pages/6273073381/Logs+to+Disk+-+Payload+Journaling+Design
    why: Documents auditor offset tracking, backpressure drop points, and the catch-up problem.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/4437541188/RFC+Logs+Agent+Distributed+Senders
    why: Per-pipeline concurrency model; shared-sender proposal.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6782419701/RFC-+Logs+Agent+Backpressure+Status
    why: Pipeline stages, backpressure propagation, rotation-related log loss.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6505529378/Adaptive+Sampling+Architecture+and+Overview
    why: Adaptive sampling credit/token design.
---

# Wildcard Evaluation — Logs Pipeline Property Catalog

**Evaluator role:** Find what the other three lenses (Antithesis Fit, Coverage
Balance, Implementability) miss: framing problems, perspective gaps, cross-lens
tensions, and systemic anomalies.

---

## F1 — CRITICAL: Fakeintake Always Stores Payloads Before Returning the Override Code (Catalog-Wide)

**Scope:** Catalog-wide; directly impacts `permanent-error-no-retry`,
`retryable-no-retry-after`, `auditor-offset-safety`, `no-loss-and-duplicate-same-line`,
`at-least-once-no-loss`, `backpressure-no-rotation-loss`.

**Concern — Systemic framing error.** The catalog's A6 assumption states fakeintake
"records all received payloads with timestamps and HTTP response codes" and that
properties can use fakeintake counts to detect whether the agent *delivered* a
payload. This is false in a specific and important way: fakeintake stores the
payload body **unconditionally** before it checks and returns the configured
status-code override.

**Evidence (code).** `test/fakeintake/server/server.go`, `handleDatadogPostRequest`:

```go
// line 469 — stores BEFORE reading the override:
err = fi.store.AppendPayload(req.URL.Path, apiKey, payload, encoding, contentType, collectTime)
// ...
// line 477 — only then returns the override status code:
if response, ok := fi.getResponseFromURLPath(http.MethodPost, req.URL.Path); ok {
    writeHTTPResponse(w, response)
    return nil
}
```

When the workload drives fakeintake to return 400, the agent receives 400,
classifies the payload as a permanent drop, does NOT retry, and advances the
auditor offset. Meanwhile fakeintake has *already stored the payload* in its
aggregator. The workload therefore sees the payload at fakeintake **even for
"permanently dropped" data**.

Consequences:

1. **`permanent-error-no-retry`** cannot be validated by checking "fakeintake
   sees the payload exactly once per 4xx" — it will see exactly once whether or
   not the agent retried, because fakeintake recorded it first.
2. **`no-loss-and-duplicate-same-line`** relies on detecting whether a
   "4xx-rejected line reappears after restart." Since fakeintake already stored
   the payload on first POST, any restart-replay POST also gets stored — the
   workload cannot distinguish "initial delivery during 4xx" from "replay
   delivery after restart" by payload body alone.
3. **`auditor-offset-safety`** tries to correlate offset-in-registry against
   delivered payloads, but fakeintake does not record whether the response it
   sent was 2xx or 4xx. The Payload struct in `test/fakeintake/api/api.go`
   has no `HTTPStatusCode` field; the aggregator's `ParseLogPayload` likewise
   carries no status code.
4. **`at-least-once-no-loss`** and loss-counting properties that count
   fakeintake entries as "delivered" will over-count — items the agent
   permanently dropped (4xx) still appear in fakeintake.

**Suggested action.** Either (a) extend the fakeintake Payload struct with the
response code actually returned (`HTTPStatusCode int`) and record it after the
override lookup (checking whether the override was applied and to what code), or
(b) completely reframe the 4xx-related properties to avoid fakeintake as the
delivery oracle, using SUT-side telemetry (`DestinationLogsDropped` counter +
auditor offset file) as the ground truth instead. Option (a) is a fakeintake
extension; option (b) changes the property formulation. Failing to address this
makes the 4xx family of properties silently vacuous or misleading.

---

## F2 — The Catalog Conflates "Per-Source Ordering" With a Guarantee the SUT Doesn't Actually Make for Production Configs

**Scope:** `per-source-ordering-preserved`; also touches `at-least-once-no-loss`
framing.

**Concern — Framing problem.** The SUT analysis (§4, S1) correctly notes that
per-source ordering is broken by two distinct mechanisms (pipeline failover,
rotation pipeline reassignment), and says "neither caveat is documented." The
catalog then frames `per-source-ordering-preserved` as a **Safety** property with
`Always`. But the question is whether this invariant is the right one to assert
given the actual delivered guarantee:

- `pipeline_failover.enabled` defaults to **false** in all production configs
  (confirmed in catalog open questions).
- Rotation pipeline reassignment is structural and can only be fixed by adding
  sequencing between old and new tailers, which is not planned.

In other words: for all production deployments without failover, the real
guarantee is **"per-source ordering within a single tailer session is
preserved"** — a weaker but actually-held invariant. The stronger "Always"
formulation that includes rotation boundaries is *false by design*, not a latent
bug. An Antithesis run will find this "bug" reliably, producing a finding that
correctly identifies the code path but has no remediation — because there is no
fix planned and it is not treated as a guarantee in any documented sense.

**Wildcard question the other lenses miss:** Should the catalog assert the weaker
real guarantee (per-session ordering) and instrument `Sometimes` that the
rotation pipeline reassignment is exercised as a known-limitation path, rather
than asserting `Always` on the stronger guarantee that is structurally false?
Asserting the structurally false `Always` burns Antithesis runtime finding an
unfixable known issue.

**Suggested action.** Reformulate: assert `Always` on within-session ordering
(new `per-session-ordering-preserved`), and add `Sometimes("rotation-pipeline-
reassignment-taken")` to confirm the path is exercised. Document that cross-
rotation ordering is a known-unsupported pattern.

---

## F3 — Processor Render/Encode Errors Are a Loss Path with No Offset Accounting — Not Covered

**Scope:** `logs-not-modified-in-transit`, `at-least-once-no-loss`; unlisted
mechanism.

**Concern — Missing perspective.** `sut-analysis.md §7` lists this as drop path
#6: "Processor render/encode error drops the message — log.Error only, no metric."
The catalog does not have a property for this path. The batch-encode property
(`batch-encode-failure-no-silent-batch-loss`) covers the *batch-layer* encode
error — but there is a separate, earlier render/encode step in the processor
itself (`processor.go:197-214`).

Specifically:
- `msg.Render()` failure → `return` with no message forwarded, no metric, no
  offset advance. The tailer's offset *does not advance* for this message, so
  it will be re-read on restart and hit the render error again in a loop.
- `p.encoder.Encode(msg, ...)` failure → same: `return`, no forward, no metric.

Neither of these is reachable under normal Antithesis faults (they require
corrupted message content or bad encoder state), so they may be in the
`Unreachable`/structural category. However, the audit trail gap (the message is
silently dropped with no counter, no offset advance, no observable signal) is
qualitatively different from the batch-encode case and worth a separate property:
`processor-encode-no-silent-loss` — structural `AlwaysOrUnreachable` that the
processor never drops a message without either forwarding it to the auditor or
incrementing a dedicated drop counter.

Additionally, a CPU-throttle fault can expose a timing window where a message in
the processor's `outputChan` buffer is not forwarded before a tailer rotation
closes the old tailer's channel — the batch-encode property covers the batch
layer, but the processor-layer drop has no SUT-side assertion at all.

**Suggested action.** Add a structural `AlwaysOrUnreachable` at both render and
encode error return sites in the processor, ensuring neither silently loses a
message. This is instrumentable as a `Sometimes("processor-render-error-dropped")`
acting as a bug trap if the path is ever reached under fault injection.

---

## F4 — The Catalog Is Over-Indexed on File Tailing; k8s/Containerd Is the Dominant Production Path and Has Distinct Failure Modes

**Scope:** Catalog-wide framing; specifically `multiline-not-split-across-pipelines`,
`at-least-once-no-loss`, `container-identifier-no-collision`,
`container-collect-all-startup-race`.

**Concern — Missing production perspective.** The SUT analysis states file tailing
is "the primary scenario." But Kubernetes with containerd is the dominant production
deployment for this agent, and the Kubernetes log path has a distinct parser
(`pkg/logs/internal/parsers/kubernetes/kubernetes.go`) with its own correctness
requirement: the **partial-flag reassembly protocol** (`P` vs `F` flags in the
containerd log format).

The `MultiLineParser.process()` in `pkg/logs/internal/decoder/line_parser.go`
handles the containerd `IsPartial` flag by buffering until the full (`F`) flag
arrives. This is a separate multiline mechanism from the user-configured `multiline`
aggregation — and it interacts with the same flush timer and the same `outputChan`
backpressure. Under fault injection:

1. A partial (`P`) chunk arrives, starts the buffer.
2. A CPU pause or backpressure stalls the tailer.
3. The flush timer fires (`flushTimeout`) — this emits the *partial* content as
   a complete message even though no `F` flag was received.
4. The `F` chunk arrives later as a new message, creating a split.

This is structurally identical to the `multiline-not-split-across-pipelines`
scenario, but via a different code path — the `MultiLineParser` flush vs. the
user-level multiline aggregator flush. The catalog does not distinguish these two
paths, and the containerd-partial-flag variant is not in the workload design
(the workload writes plain log lines to a shared file volume, not containerd-
format `P`/`F`-flagged lines).

More broadly: the container identity collision FIXME (`tailer.go:259`) and the
container churn properties (`container-collect-all-startup-race`,
`container-addremovesource-ordering`) are listed but the topology explicitly
excludes container sources from the default topology ("No fourth container is
needed"). All three container-identity properties are therefore **vacuous by
topology design** in the default run.

**Suggested action.** Either (a) add a second topology variant with container
sources enabled (Docker socket), which exercises the identifier collision, the AD
race, and the containerd partial-flag multiline correctly, or (b) explicitly
label the container-source cluster as "requires topology extension" and remove
them from the default catalog run. Keeping them in the default catalog makes them
appear covered when they are not exercised.

---

## F5 — Fakeintake Has No Deduplication; "Duplicate Detection" Properties Assume Intake Dedup That Doesn't Exist Here

**Scope:** `no-loss-and-duplicate-same-line`, `at-least-once-no-loss`,
`auditor-drains-on-stop`.

**Concern — Mock vs. real intake semantic gap.** The real Datadog intake
deduplicates some log payloads. Fakeintake does not deduplicate at all — it
stores every payload body it receives. This has an asymmetric effect:

- In production, a crash-and-replay cycle that sends duplicate logs may result
  in zero visible duplicates at the backend (if dedup key matches). The user
  experience is "at-most-once with replay."
- In the Antithesis run, the same crash-and-replay produces observable duplicates
  at fakeintake. Assertions that catch duplicates are meaningful at fakeintake
  but may not represent a user-visible failure.

The catalog frames `at-least-once-no-loss` correctly as "duplicates OK; absent
ones are not" — which is the right contract at the agent side. But `no-loss-and-
duplicate-same-line` uses the absence of a line *combined with* the presence of
another as a signal. Since fakeintake never deduplicates, any re-delivered line
after a crash is trivially visible — which makes the "duplicate" half of this
property noisier than expected. The property may generate many false positives for
benign replay behavior.

**Suggested action.** The property is still correctly formulated at the *agent
contract* level (the agent must not both lose one offset range and duplicate
another in a single session). The concern is labeling: the property should make
explicit that "duplicate" is measured at the fakeintake received count, not the
backend visible count — so a benign replay (same content, second time) must be
distinguishable from a pathological duplicate (different session, same offset
range). The workload sequence number is the right distinguishing key; the property
writeup should document that the sequence number is the dedup key, not payload
body.

---

## F6 — "Corruption" as a Failure Mode Is Nearly Absent From the Catalog; All the Loss Properties Treat Bytes as Inert

**Scope:** `logs-not-modified-in-transit`, and the catalog as a whole.

**Concern — Systematic bias toward loss, away from corruption/wrong-metadata.**
The catalog has 12 properties about *loss* (data absent from intake) and 3 about
*ordering* (data present but sequenced wrong), but only 1 about byte-level
*corruption* (`logs-not-modified-in-transit`) and only 1 about *metadata
corruption* (`container-collect-all-startup-race`). This is asymmetric with the
actual risk profile:

1. **Metadata corruption is higher-probability than byte corruption.** The tag,
   service, hostname, source, and status fields on a log message are set by the
   Origin/metadata layer and can be wrong even when the content bytes are correct.
   The processor's `filterMRFMessages` assigns `IsMRFAllow` based on `msg.Origin.
   Service()` — if Origin is wrong (from the container collect-all race), the MRF
   routing is wrong, silently sending data to the wrong regional endpoint. There
   is no property for "MRF routing correctness."

2. **Processor pipeline ordering (Exclude/Include filter → MRF tag → encode)**
   means that a message excluded by `ExcludeAtMatch` is correct; but the same
   message that survives `ExcludeAtMatch`, gets MRF-tagged, then encode-errored,
   produces a *metadata-tagged-but-content-lost* payload. No property covers this
   cross-stage interaction.

3. **Kubernetes parser errors produce wrong status fields.** When the `kubernetes`
   parser cannot parse the header (timestamp/stream/flag), it returns the raw
   content with `StatusInfo` default — meaning an `stderr` error line arrives at
   the intake tagged as `INFO`. `secrets-redacted-before-send` covers content
   integrity but there is no property for "log level/status field correct in
   transit."

4. **Tag accumulation in AdaptiveSampler.** When a high-match-count entry emits
   its `adaptive_sampler_sampled_count` tag, the tag is appended to
   `msg.ParsingExtra.Tags`. If the adaptive-sampler is later disabled (or the
   entry is evicted and a new one created), the tag persists on the message object.
   No property checks that the `ddtags` field in the fakeintake payload does NOT
   contain stale sampler tags from a previous sampling state.

**Suggested action.** Add at minimum one "metadata integrity" property:
`log-metadata-not-corrupted` — workload asserts that every received line at
fakeintake has (a) the expected `ddsource`, `service`, `hostname` fields, and (b)
no unexpected tags from prior processing states. This complements
`logs-not-modified-in-transit` (which only checks content bytes) with a check
on the envelope.

---

## F7 — MRF / Dual-Shipping Is a Live Production Feature With No Property Coverage

**Scope:** Catalog gap; unlisted mechanism.

**Concern — Missing perspective.** Multi-Region Failover (MRF) is a live,
production-enabled feature (`multi_region_failover.failover_logs`) that adds a
second, parallel send path to an MRF destination. The topology makes no mention
of it; the catalog has no property for it. The MRF path:

- Adds an `IsMRF` destination that receives non-blocking sends (via
  `NonBlockingSend`, per `worker.go:169-175`).
- The worker drops MRF-bound payloads silently if the `DestinationSender.input`
  (capacity 10) is full — **this is an invisible loss path for MRF data.**
- The processor's `filterMRFMessages` runs *after* `Render()` but *before*
  `Encode()` — so the MRF tag is set on pre-encoded content. If the encode step
  fails after `IsMRFAllow = true`, the MRF destination never receives the payload
  (no double-send protection), but the offset still advances.

The SUT analysis mentions MRF only implicitly (as "secondary destinations,
`NonBlockingSend`"). No catalog property covers: does the MRF destination
actually receive the same set of payloads as the primary destination? Does a
transient MRF destination failure (its intake returning 5xx) silently drop MRF-
bound payloads while primary continues normally?

This matters most because MRF is designed exactly for disaster-recovery scenarios
— the correctness of MRF delivery is most at risk under the same fault conditions
Antithesis exercises. If the topology includes a secondary fakeintake for MRF
(mentioned as "optional" in deployment-topology.md), the catalog should have a
property for it.

**Suggested action.** If MRF is in scope for the test environment, add
`mrf-delivery-consistent-with-primary` — `Always`: for any payload reaching the
primary fakeintake, the MRF fakeintake receives the same payload within the
`NonBlockingSend` best-effort bound; and `Sometimes("mrf-drop-on-full-buffer")`
to confirm the known-drop path is exercised. If MRF is out of scope, the
topology should explicitly disable it to avoid contaminating the property results.

---

## F8 — The Sampler's `Exclude` Filter Ordering Means "Excluded = Always Sent" — The Open Question in the Catalog Is Backwards

**Scope:** `high-value-never-sampled` open question.

**Concern — Framing error in open question.** The catalog's open question for
`high-value-never-sampled` asks: "Do `Exclude` filters take precedence over
protection (`shouldSample` before `isImportant`), and is that intended?"

The code answers this definitively. In `sampler.go` `shouldSample()`:
```go
func (s *AdaptiveSampler) shouldSample(msg *message.Message, tokens []Token) bool {
    if matchesAnyFilter(s.config.Exclude, msg, tokens, s.config.MatchThreshold) {
        return false  // Exclude = "don't rate-limit this"
    }
    ...
}
```

`shouldSample=false` means the message goes through without rate-limiting (returned
at `Process()` line 190-192, before `isImportant` is checked). So `Exclude` is
"never rate-limit these patterns" — it *protects* messages from sampling, it does
not *exclude them from delivery*. An "important" FATAL log that also matches an
`Exclude` filter is double-protected.

The catalog open question implies `Exclude` might *drop* important logs — which is
wrong. This should be closed with "confirmed: Exclude bypasses rate-limiting
entirely; important logs that also match Exclude pass trivially; no conflict."

**Suggested action.** Close this open question in the catalog with the correct
analysis. The framing "Exclude takes precedence over protection" inverts the
semantics — `Exclude` IS the broadest form of protection.

---

## F9 — Adaptive Sampler Is Labeled Experimental; Testing It May Be Validating a Non-Default, Potentially Deprecated Path

**Scope:** Category D (all 5 sampling properties); assumption A2.

**Concern — Framing question.** The config key is
`logs_config.experimental_adaptive_sampling` (note: `experimental_`). The decoder
resolution in `decoder.go:156` calls `resolveAdaptiveSamplerEnabled`, which checks
`sourceAdaptiveSampling` (a per-source config struct). No global default enables
it. The catalog correctly notes (A2) that the whole cluster is vacuous without
explicit enablement.

The broader concern: if this feature is `experimental` and not GA, the catalog
spends 5 of 36 properties (14%) on a code path that may be: (a) not deployed in
any production config, (b) likely to be refactored before GA, making the
SUT-side assertions stale. The owner's design doc (cited) proposes property
testing specifically for sampling, suggesting it *is* in active development — but
"in active development" also means the properties are likely to need updating
before the feature stabilizes.

**Suggested action.** Confirm with the feature owner whether `experimental_
adaptive_sampling` has any production deployments. If not, deprioritize Category D
for the initial run and gate it explicitly. The 5 properties represent significant
instrumentation investment for a non-default code path. If the feature is heading
toward GA, this is the right time to invest — but the evaluation should note the
risk.

---

## F10 — The Workload's "Sequence Number in Every Log Line" Assumption May Break the Kubernetes Log Parser

**Scope:** Workload design assumption; cross-cuts all properties using sequence
numbers at fakeintake.

**Concern — Missing perspective.** The topology's workload embeds a per-source
sequence number in each log line (per deployment-topology.md, §Client). However,
the Kubernetes containerd log format wraps arbitrary content with a header:
`<timestamp> <stream> <flag> <content>`. If the workload writes plain text files
with embedded sequence numbers, the Kubernetes parser is not invoked (the agent
sees them as plain file-tailed lines, not container logs). This is consistent and
correct for the default topology.

But if the topology is extended to add container sources (see F4), the sequence
number format will be wrapped inside the Kubernetes header, and the fakeintake
aggregator (which only stores the decoded `message` field) will strip the header
before the workload can read the sequence number back. The workload reconciliation
logic would need to be containerd-aware.

More subtly: the `kubernetes.go` parser returns an error for unparseable headers
but still returns the message with raw content (line 53: `message.NewMessage(msg.
GetContent(), nil, status, 0), errors.New(...)`). A partially-parseable line (e.g.
with a bad timestamp) bypasses header stripping, so the sequence number is present
but the message metadata (status, partial flag) is wrong. The workload cannot
distinguish "correct parse" from "fallback parse" from the received content alone.

**Suggested action.** Document this cross-cut explicitly in the workload design.
If container sources are added (F4), the workload must write containerd-format
lines (with valid timestamp/stream/flag headers), and the sequence number must be
in the `<content>` portion. Reconciliation should work; the issue is only
if someone adds container sources without updating the workload format.

---

## F11 — The Clock Fault's Monotonic-Clock Question Is a Single-Point Gate That Blocks an Entire Cluster

**Scope:** All of Cluster 7 (clock-sensitive properties); possibly Cluster 4.

**Concern — Cross-lens tension (Fit says high-value, Implementability says
infeasible).** Four catalog properties (`clock-jump-no-backoff-underflow`,
`clock-jump-no-extra-sampling`, `multiline-not-split-across-pipelines` clock
variant, `container-identifier-no-collision` timestamp guard) all share the same
unresolved open question: "Does the Antithesis clock fault move Go's monotonic
clock or only wall-clock?"

Go's `time.Now()` returns a struct carrying both monotonic and wall-clock
readings. `time.Since()`, `time.Sub()`, `time.After()` use the monotonic
component when both operands have one. The SUT's critical timing paths all use
monotonic arithmetic:
- Backoff guard: `blockedUntil.After(time.Now())` — both have monotonic.
- Sampler credit refill: `now.Sub(e.lastSeen).Seconds()` — both from
  `time.Now()`, use monotonic.
- EWMA: `time.Since(virtualLatencyLastSample)` — uses monotonic.

If the Antithesis clock fault only moves the wall-clock (the RTC/CLOCK_REALTIME),
none of these code paths are reachable. All clock-sensitive properties become
**vacuously true** — not because the property holds, but because the fault never
triggers the code path. The `Sometimes` reachability assertions would never fire,
making the run silently uninformative about clock correctness.

If the Antithesis clock fault moves CLOCK_MONOTONIC as well (which would be
unusual but is possible in a virtualized environment), the paths are all
reachable.

**Suggested action.** This must be resolved with the Antithesis tenant before
any clock-sensitive properties are included in the catalog. If the fault is
wall-clock only, reformulate the clock properties to use `time.Sleep`-based
timing (which uses CLOCK_MONOTONIC internally but is bounded by wall-clock
elapsed time on some platforms) or simply drop Cluster 7 and document why.
A reformulation: rather than relying on the clock fault to trigger `elapsed` in
the sampler, inject a controlled `now` function (already exists as a test
injection point in `sampler.go`) via a SUT-side configuration knob that the
workload can toggle — making this a custom-fault property instead of a clock-
fault property.

---

## F12 — The Auditor TTL Cleanup Is an Unmodeled Hazard for Long Soak Runs

**Scope:** `registry-recovers-after-crash`, `at-least-once-no-loss`; long-run
topology concern.

**Concern — Missing perspective.** The auditor cleans up registry entries with a
TTL of approximately 23 hours (`cleanUpTTL`) every 300 seconds. Antithesis runs
can span many simulated hours (in terms of fault density, not real time). If the
simulated time includes a long no-activity period for a source — which is
plausible under prolonged network partition — the auditor may evict that source's
registry entry. On next file activity (after fault recovery), the tailer calls
`GetOffset()`, gets a miss, and starts from the default `TailingMode` position
(usually EOF). Data from before the partition is now permanently undeliverable —
and this is indistinguishable from the normal "catch up from EOF" behavior on a
new source.

This cleanup is intentional and correct for sources that genuinely disappear. But
in a fault-injected run, a source under sustained partition is not gone — it's
blocked. If the topology uses short `close_timeout` values to speed up the
rotation-loss scenario, the combination of short timeout + long simulated
partition + normal TTL cleanup could silently corrupt the at-least-once guarantee
in a way that looks like normal operation.

The catalog does not model this. The registry-related properties assume the TTL
is longer than any fault window in the test.

**Suggested action.** Pin `registry_ttl` in the topology configuration to a value
significantly longer than the longest expected fault window, or measure the
actual elapsed-time span of the Antithesis run and verify no TTL evictions occur
during the test. Add a SUT-side `Reachable("registry-entry-ttl-evicted")` as a
bug trap — if it fires, the test's at-least-once invariants are compromised.

---

## Passes (Findings Not Raised)

**P1 — Cluster structure is sound.** The 9-cluster grouping in
`property-relationships.md` correctly identifies shared fault levers and
dominance relationships. The suggestion to run `sampling-reachable-under-load`
as a gate for Cluster 4 is correct and prevents vacuous passes.

**P2 — `Exclude` filter semantics are correctly tested in `high-value-never-
sampled`.** The property `Always` that FATAL/ERROR lines are delivered is
correctly formulated because `Exclude` (which bypasses rate limiting) does not
help important logs — both `Exclude` and `isImportant` pass them through, but
neither can drop them.

**P3 — The `Services` deadlock dormancy call-out is accurate.** The catalog
correctly notes no production subscribers exist for the deadlock-loop code.
Framing it as a regression guard is the right characterization.

**P4 — The `at-least-once` exception list is honest.** The documented exceptions
(4xx permanent drop, rotation closeTimeout, seek-error reset) are correctly
listed. A well-crafted `at-least-once` property that accounts for these avoids
asserting an invariant the SUT intentionally violates.

**P5 — `Seek` error silent-zero is correctly flagged as a distinct hazard.** The
dual-error: the Windows tailer has the same bug at `tailer_windows.go:37`.
The catalog notes this and correctly identifies it as cross-platform — good
evidence gathering.

**P6 — Fakeintake `ResponseOverride` mechanism is confirmed usable** for
driving specific HTTP status codes per-endpoint. The mechanism exists and works
as needed for protocol-contract properties — the concern in F1 is specifically
about payload storage ordering, not the override mechanism itself.

---

## Uncertainties

**U1 — Whether MRF is enabled in any planned test topology.** The topology doc
says a second fakeintake is "optional." If MRF is silently enabled but the second
fakeintake is absent, the worker's `NonBlockingSend` to the MRF destination
silently drops everything with no metric. This is neither a pass nor a finding
without knowing the topology's MRF configuration.

**U2 — Whether the Antithesis clock fault moves Go's monotonic clock.** Covered
in F11 but genuinely uncertain — requires tenant input. Affects reachability of
the entire Cluster 7.

**U3 — Whether `experimental_adaptive_sampling` has production deployment.** If
yes, Category D is high-priority. If no, it may be a future-proofing investment
that is not yet worth the implementation cost.

**U4 — Whether the "trimmed logs-only binary" build option (topology open
question) changes any property reachability.** A trimmed binary might not include
the MRF processor code, the journald launcher, or the container launcher —
changing which properties are even relevant.

**U5 — Fakeintake TTL and flush behavior during long soak.** The in-memory store
has a `CleanUpPayloadsOlderThan` method called on a ticker. If the store flushes
old payloads before the workload reconciles, the at-least-once check could
produce false negatives. The catalog assumes the workload reconciles within the
retention window but this is not pinned.

---

## Cross-Lens Tensions Identified

| Tension | Fit says | Implementability says | Wildcard resolution |
|---------|----------|----------------------|---------------------|
| Clock properties | High Antithesis value (timing-sensitive) | May be infeasible if wall-clock only | Resolve monotonic-clock question first; reformulate as custom-fault if wall-only (F11) |
| 4xx-related properties | High value (protocol correctness) | Feasible given fakeintake override | Fakeintake always stores before responding — all 4xx properties need redesign (F1) |
| Container properties | Medium value | Topology excludes container sources | Properties are vacuous by design in default run (F4) |
| Sampler cluster | Owner-requested | Experimental config, may not be in GA | Prioritization question pending deployment confirmation (F9) |
| Weaker ordering guarantee | High value catching real bugs | `Always` formulation is structurally false | Reformulate to assert real guarantee (F2) |
