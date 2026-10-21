# Automatic periodic check metric accumulation

This experimental implementation accumulates check metrics automatically and
drains eligible metric types on the Agent's existing flush cadence (normally
about 15 seconds). It starts from the independent periodic branch, without the
size-based sender/rtloader machinery or per-submission Python GIL release.

The aggregator input loop waits for streaming serialization to finish. Internal
default-sender numeric telemetry bypasses that paused input channel, and the
aggregator mutex is released before writing to the streaming sink. These are
necessary exceptions to literal global blocking: serializer/forwarder feedback
must not wait on its own input loop.

## Metric behavior

| Metric | Periodic behavior | Check completion |
| --- | --- | --- |
| Gauge | Emit latest value; reclaim flushed metadata | Commit normally, or defer a same-second collision |
| Count | Emit accumulated contributions | Same; deferred contributions remain accumulated |
| Monotonic count | Emit deltas, preserve resets and baseline | Same; baseline retains run-based expiry and additional timeout |
| Explicitly timestamped gauge/count | Emit supplied points without changing timestamps | Drain remaining supplied points when eligible |
| Rate | Retain the original last-two-samples behavior | Flush at check completion |
| Histogram/historate | Preserve the complete check-run distribution | Flush aggregates/percentiles at check completion |
| Set | Preserve uniqueness across the complete run | Flush cardinality at check completion |
| Distribution/bucket | Flush closed timestamp buckets, preserve bucket baselines | Keep open buckets until a later flush; drain all buckets on shutdown |

Preserving run-scoped metrics is a deliberate qualification of "flush everything
every 15 seconds." It avoids silently changing histogram averages/percentiles,
historate windows, rates, and set uniqueness. It also means histogram-heavy
checks retain full-run histogram state. Gauge/count emission cadence changes;
there is no claim of universal equivalence for every downstream query.

## Final commits and clocks

Generated numeric points must not collide with an earlier commit's wire second.
A final commit in that second closes run-scoped metrics immediately, but leaves
eligible numeric contributions pending. There is no sleep, timer wait, new
goroutine, or held-GIL wait for the wall clock. Deferred context cleanup cannot
discard these contributions. A later sample in a later wall-clock second first
flushes the pending numeric state so it cannot overwrite the previous run's
gauge. Otherwise the next periodic flush drains it.

Repeated values within one unresolved wire second can coalesce: counts add,
gauges retain their latest value. This is not a guarantee of distinct gauge
points for checks completing faster than the wire's one-second resolution.

On backward wall-clock movement, periodic flushes use the last generated numeric
commit second plus one. A same-second final shutdown uses the same rule. This
preserves ordering and progress without waiting or retaining an outage-sized
backlog. It does **not** repair the machine's clock: a large rollback can produce
future-dated numeric points that an intake may reject. Rollback is logged with
throttling. Forward clock jumps use the new wall time. No explicit sample or
sketch timestamp is rewritten; ordinary sketch closure uses actual wall time.
Run-scoped metrics retain their existing completion timestamp semantics.

## State and memory

After an intermediate flush, gauges/counts and explicitly timestamped metrics
release context metadata. Monotonic metrics also release names/tags, retaining
the existing metric object and a compact context-key/run-index entry. Recreating
metadata on the next sample reuses the baseline. Baseline timeout starts only
after the original completed-run expiry threshold, not on every periodic tick.
Bucket baselines and open sketches keep their associated contexts.

Time-based flushing does not impose a hard cardinality or byte limit. Retained
baselines, a very high arrival rate, run-scoped histograms, and detached output
still consume memory. A peak-memory improvement must be measured, not inferred
from metadata reclamation alone. Expiration-disabled configurations retain their
existing metric-lifetime behavior.

## Shutdown and backpressure

The shared bounded accepted-input drain runs before synchronous final flushing.
Final flushing drains pending numeric state and every accepted sketch bucket,
including buckets ahead of wall time following a clock rollback. Sketch point
timestamps remain unchanged. A producer submitting after the captured shutdown
drain boundary is outside that guarantee.

Serialization completion means handoff to the forwarder, not HTTP delivery.
Forwarder queues, retries, capacity limits and shutdown timeouts remain in force.
An arbitrarily long intake outage is not lossless. Global serialization can fill
the finite input queue and block a Python submitter while it holds the GIL;
unrelated-check responsiveness must be measured. No architectural lower bound
equal to the earlier prototype's measured pause is claimed.

## Validation and rollback

Use the focused sampler/sender regressions and full aggregator/metrics suites,
including race mode, before native intake tests. Validate final values and
histogram query values in addition to count conservation. Keep raw receipts and
identify exact retransmissions separately from distinct-payload point collisions.

This experimental branch has no per-check opt-in or runtime toggle. Roll back by
deploying the prior Agent build. Native Linux validation does not establish
Windows/macOS packaging or lifecycle coverage, or constitute release approval.
