# ADR-003: Bound response writes before waiting for capture teardown

- **Status:** Accepted (reconstructed from author-confirmed PR decisions; not a new
  project approval)
- **Date:** 2026-10-22 (formalized from existing design)
- **Affects:** REQ-PCAP-004, REQ-PCAP-005, REQ-PCAP-006

## Context

The capture’s duration and request context can expire while its packet-writing goroutine
is blocked on a caller that leaves a socket open but stops reading.
Waiting for that goroutine in `Stop` does not release the capture: cancellation does not
interrupt the blocked response write.
A reviewer demonstrates why a “bounded capture” requires a bound at the output boundary
too.

## Options considered

1. **Rely on the duration, context cancellation, and subsequent Stop.** Bounds normal
   packet reading, but cannot unblock an outstanding write and therefore can strand
   capture teardown.
2. **Set a response write deadline before starting the capture.** Allows blocked writes
   to fail and the drain loop to exit, while reserving grace for stopping and final
   trailers. Depends on transport support for write deadlines.

The review also permits other mechanisms that unblock writes; it does not document a
detailed alternative cancellation design.

## Decision

Set a response write deadline before starting capture, at the requested duration plus
the stop grace and trailer grace (five seconds each).
The author explicitly confirms this fix in the pinned head commit.
Keep request cancellation and capture duration as complementary controls.

The requirement evolves from “collection has a duration” to “an unresponsive consumer
cannot indefinitely prevent teardown.”
The chosen deadline mechanism belongs here; the observable bound belongs in the
requirements.

## Consequences

- **Positive:** A stalled reader cannot indefinitely hold the supported HTTP transport’s
  capture-writing goroutine and capture handle.
- **Negative:** Slow consumers can lose a capture when the deadline fires.
  Writers without deadline support remain unbounded by this mechanism.
- **Neutral:** The full client round-trip timeout includes a separate 30-second setup
  allowance; it is not the same deadline as the server’s output-write bound.

## References

- [Review explains the stalled-write teardown failure](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4317811909).
- [Author confirms the response deadline fix](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4352584973).
- `handleCapture`, `setWriteDeadline`, `capturer.Stop`, `capturer.drainLoop`,
  `socketCaptureTrigger.Capture`;
  [requirements](../remote-packet-capture/requirements.md).
- [ADR-001](001_capture-limits-cannot-be-raised.md) establishes caller-independent
  bounds; this decision closes a different path to unbounded resource use.
