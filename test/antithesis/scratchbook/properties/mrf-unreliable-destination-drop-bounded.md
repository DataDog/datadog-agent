---
slug: mrf-unreliable-destination-drop-bounded
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# mrf-unreliable-destination-drop-bounded — MRF Unreliable-Destination Drop Is Bounded and Does Not Affect Primary Delivery

> **Topology-extension required**: this property requires a second intake
> (secondary/MRF region) and `multi_region_failover.failover_logs = true` in the
> agent configuration. The standard single-intake topology does not exercise the
> MRF unreliable-destination path.

## What Led to This Property

`sut-analysis.md` §4 (S5) states: "Unreliable-destination failures don't block
the pipeline or advance the auditor. Enforced via `noopSink` + `NonBlockingSend`
(silent drop on full buffer)." §7 (failure mode #1) identifies the drop path:
"`NonBlockingSend` drop for secondary reliable destinations on full buffer
(`sender/worker.go:160-164`) — counter only."

No existing property covers the MRF-specific unreliable-destination path. This
property targets the `worker.go:168-183` block where MRF-bound payloads are
sent to unreliable destinations via `NonBlockingSend`. When the unreliable
`DestinationSender.input` channel (capacity 10 per `logs_config.payload_channel_size`)
is full, the payload is silently dropped with only a `tlmPayloadsDropped` counter
increment. The property has two components:

1. **The drop is bounded**: drops from the unreliable destination channel never
   overflow into the primary (reliable) destination path. The primary destination
   always receives the payload regardless of unreliable-destination backpressure.

2. **The drop is observable**: `tlmPayloadsDropped` is incremented (counter only,
   no alert by default), and the primary at-least-once guarantee is preserved
   (auditor offset advances normally after primary destination 2xx).

## Code Paths Involved

**`comp/logs-library/sender/worker.go:101-110`** — destination setup:

```go
func (s *worker) run() {
    noopSink := noopDestinationsSink(s.bufferSize)
    reliableOutputChan := s.outputChan
    if reliableOutputChan == nil {
        reliableOutputChan = noopSink
    }
    reliableDestinations := buildDestinationSenders(s.config, s.destinations.Reliable, reliableOutputChan, s.bufferSize)
    unreliableDestinations := buildDestinationSenders(s.config, s.destinations.Unreliable, noopSink, s.bufferSize)
    ...
}
```

Reliable destinations write to `s.outputChan` (the auditor input). Unreliable
destinations write to `noopSink` (a drained channel; acks are never forwarded
to the auditor). This structural separation is the primary guarantee.

**`comp/logs-library/sender/worker.go:167-183`** — MRF unreliable destination send:

```go
// Attempt to send to unreliable destinations
for i, destSender := range unreliableDestinations {
    // Drop non-MRF payloads to MRF destinations
    if destSender.destination.IsMRF() && !payload.IsMRF() {
        log.Debugf("Dropping non-MRF payload to MRF destination: %s", destSender.destination.Target())
        sent = true
        continue
    }
    if !destSender.NonBlockingSend(payload) {
        tlmPayloadsDropped.Inc("false", strconv.Itoa(i))
        tlmMessagesDropped.Add(float64(payload.Count()), "false", strconv.Itoa(i))
        ...
    }
}
```

When `NonBlockingSend` returns `false` (buffer full), `tlmPayloadsDropped` is
incremented with `reliable="false"` (unreliable destination). The payload is
dropped. The loop continues to the next destination — it does not block or
affect the reliable destination flow.

**`comp/logs-library/sender/destination_sender.go:132-141`** — `NonBlockingSend`:

```go
func (d *DestinationSender) NonBlockingSend(payload *message.Payload) bool {
    select {
    case d.input <- payload:
        return true
    default:
    }
    return false
}
```

`d.input` has capacity `bufferSize` (= `logs_config.payload_channel_size`, default
10). When full, the default branch fires and the payload is dropped silently.

**`comp/logs-library/sender/worker.go:120-148`** — reliable destination send loop:

```go
sent := false
for !sent {
    for _, destSender := range reliableDestinations {
        if destSender.destination.IsMRF() && !payload.IsMRF() {
            sent = true
            continue
        }
        if destSender.Send(payload) {
            sent = true
            ...
        }
    }
    if !sent {
        time.Sleep(100 * time.Millisecond)
    }
}
```

The reliable destination loop spins until `destSender.Send` returns `true`. It
does not interact with the unreliable destination state. The two loops are
sequential but independent: the unreliable loop runs *after* the reliable loop
completes. A full unreliable-destination buffer cannot block the reliable send.

**Structural separation between reliable and unreliable paths:**

The entire unreliable-destination code block (lines 167-183) runs only *after*
the reliable send loop (lines 120-148) has succeeded. The `sent = true`
condition from the reliable loop means the payload has already been queued for
primary delivery before the unreliable block executes.

## What Goes Wrong If the Property Is Violated

- **Cross-path contamination** (worst case, not currently possible by code
  structure): an unreliable-destination drop somehow prevents the reliable
  destination from receiving the payload. The auditor never sees the ack;
  at-least-once is broken.
- **Unobservable drops** (current risk): `tlmPayloadsDropped` is incremented
  but it is a raw counter with no default alert. If the MRF secondary intake is
  consistently overloaded or partitioned, the counter grows without operator
  awareness.
- **Auditor advances without MRF delivery**: the auditor advances the offset
  (based on reliable 2xx), but the MRF payload was dropped. On failover, the
  secondary region has a gap. The at-least-once guarantee applies only to the
  primary region.

## Assertion Design

**`Always`** (workload-side): For every payload written to the workload's log
source:
- Assert the payload appears at the **primary** fakeintake (primary region).
- Assert that the auditor offset advances past the payload's messages.
- The assertion is independent of whether the payload appears at the MRF
  (secondary) fakeintake — drops at the secondary are acceptable.

**`AlwaysOrUnreachable`** (SUT-side): At the point where `NonBlockingSend`
returns `false` for an MRF unreliable destination, assert that the reliable
destination has already been sent (i.e., `sent == true` at this point):

```
antithesis.AlwaysOrUnreachable(
    "mrf-unreliable-destination-drop-bounded: reliable sent before unreliable drop",
    sent == true,
    map[string]any{
        "destination": destSender.destination.Target(),
        "payload_count": payload.Count(),
    },
)
```

This is `AlwaysOrUnreachable` because the MRF topology is required to reach
this code path; without MRF enabled, the `unreliableDestinations` list is empty.

**`Reachable`** (SUT-side): At the `NonBlockingSend` drop site (inside the
`!destSender.NonBlockingSend(payload)` block), add:

```
antithesis.Reachable(
    "mrf-unreliable-destination-drop-bounded: mrf-unreliable-drop",
    map[string]any{"destination": destSender.destination.Target()},
)
```

This confirms Antithesis explored a scenario where the MRF secondary buffer was
full, making the drop path active.

**`Sometimes`** (workload-side): Assert that `tlmPayloadsDropped{reliable="false"}`
counter is observed to increment at least once during the run, confirming the
drop path is exercised under the planned fault (partition to secondary intake).

## Why It Matters

MRF deployments rely on the secondary region receiving logs during primary region
failures. Under Antithesis network partitions to the secondary intake:
- The unreliable-destination buffer (capacity 10) fills quickly.
- `NonBlockingSend` drops payloads silently.
- Operators have no real-time signal (only the raw counter, not an alert).

The primary delivery guarantee must hold throughout. The property validates:
1. The structural separation between reliable and unreliable paths is preserved
   under fault injection (code-level guarantee).
2. Drops at the secondary are observable (metric-level guarantee).
3. Drops at the secondary do not contaminate the primary at-least-once guarantee.

Without this property, a future refactor that moves the reliable-send logic
into a shared function could accidentally create a code path where an unreliable
buffer-full condition blocks the reliable send, violating at-least-once.

## Relationship to Other Properties

- `at-least-once-no-loss` — covers the primary at-least-once guarantee; this
  property is the specific MRF-secondary constraint that must not weaken it.
- `auditor-offset-safety` — the auditor only advances for primary-destination
  2xx; this property depends on that separation being maintained under MRF faults.
- `queued-payloads-eventually-sent` — covers retry/delivery for the primary
  destination; this property covers the secondary (unreliable) destination drop
  path which has no retry.

## Open Questions

- Does the planned Antithesis topology include a second intake (MRF secondary
  region), or will this property require a separate harness variant?
  `(needs human input)`
- Is `multi_region_failover.failover_logs` ever the default for any supported
  agent configuration, or is it always opt-in? If always opt-in, this property
  is topology-gated in the same sense as the journald property.
  `(partial: from processor.go:107 it reads from config; default appears to be
  false, making this strictly opt-in)`
- What is the realistic fill rate of the unreliable-destination buffer (capacity
  10) under a partitioned secondary intake? If the buffer fills faster than the
  pipeline empties it, the drop rate approaches 100%. Is this expected behavior
  or a bug? `(needs human input)`

### Investigation Log

#### Is there any code path where an unreliable-destination drop can block or affect the reliable send?

- Examined: `comp/logs-library/sender/worker.go:101-210` (full `run()` function), `comp/logs-library/sender/destination_sender.go:132-141` (`NonBlockingSend`), `comp/logs-library/sender/destination_sender.go:98-130` (`Send`).
- Found: The reliable-destination loop (lines 120-148) must complete (`sent = true`) before the unreliable-destination loop (lines 167-183) is entered. `NonBlockingSend` uses a non-blocking select; it cannot block. The `noopSink` that unreliable destinations write to is a buffered channel (capacity `bufferSize`) drained by a goroutine — it cannot block either. The structural separation is clear and unambiguous.
- Not found: any shared state between the reliable and unreliable loops that could create cross-contamination.
- Conclusion: the "drop is bounded" component of the property is structurally guaranteed by the current code. The Antithesis value is as a regression guard for future refactors and to confirm observability of drops.

#### What telemetry is emitted on an unreliable-destination drop?

- Examined: `comp/logs-library/sender/worker.go:22-26` (counter declarations), `comp/logs-library/sender/worker.go:175-178` (drop site).
- Found: `tlmPayloadsDropped.Inc("false", strconv.Itoa(i))` and `tlmMessagesDropped.Add(float64(payload.Count()), "false", strconv.Itoa(i))` are incremented. The `"false"` label corresponds to `reliable="false"`. These are dogstatsd-style internal counters (via `telemetryimpl`). No `log.Warn` or `log.Error` is emitted for individual drops.
- Not found: any default alert configured on this counter in the agent's telemetry stack.
- Conclusion: drops are observable via the counter metric, but only if the operator is monitoring `logs_sender.payloads_dropped` with label filtering. The observability is present but not prominent.
