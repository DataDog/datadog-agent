# Coordinated sampling

Live Debugger snapshots used to be sampled one probe at a time. Each probe had
its own rate limit, so a request that hit five probes could easily produce two
snapshots, and nothing in the output said the other three were missing.

Coordinated sampling makes the keep-or-drop decision once per request and
applies it to every probe in that request. You get the whole chain of snapshots
or none of it.

This covers tier 1 of the RFC "Improving Correlation for Live Debugger
Snapshots": correlate on the trace id we can already reach when a
`context.Context` is in scope. No new way of carrying trace context.

## How it works

When a probe fires, before any capture work:

1. Find the trace id. `irgen` already knows, per probe, whether a
   `context.Context` is in scope and where it lives (which registers, or which
   stack slot). The eBPF side reads the interface from there and walks the
   context chain to the dd-trace span, the same hops the capture path makes.
2. Look the trace up in `decision_by_trace`, an LRU map keyed by trace id.
   - Not there: this is the first probe of the request. Ask the session
     throttler, store the answer.
   - There: use the stored answer.
3. On keep, check the probe's bit in the record's bitset. If it is already set,
   this probe has emitted for this request, so drop. Otherwise set it and emit.
4. Kept snapshots draw from the session budget.

If there is no context in scope, there is no trace id, and the probe falls back
to its own throttler. That is the old behavior, unchanged.

## What the design chooses

- **One budget for the whole session**, not one per probe. Only the first probe
  of a request is gated by it. Later probes of a kept request emit regardless,
  which can push the budget negative and hold back the next requests until the
  period resets. Complete chains are worth more than an exact rate.
- **At most one snapshot per probe per request.** A probe inside a loop cannot
  flood the output. The cap is a bitset in the per-request record, so it costs
  no extra map.
- **Decide in eBPF, before capture.** A dropped probe pays for the context walk
  and nothing else.
- **LRU for the decision map.** Memory is bounded. A long request whose record
  is evicted mid-flight simply decides again, which can split a chain. The map
  holds 16384 requests, which makes that rare.
- **Shallower walk than capture.** The pre-decision walk follows 16 links; the
  capture walk follows 32. This one runs on every hit, including hits we are
  about to drop, so it stays cheap. A span buried deeper reads as "no trace".
- **Probe ids 256 and up skip the per-probe cap**, because the bitset is 256
  bits. They still follow the request's decision.

## Where the code is

- `ebpf/coordinated_sample.h` — the decision map, the walk, the session
  throttler accounting, the per-probe cap.
- `ebpf/event.c` — calls the walk at the top of the uprobe; both throttle points
  then go through `should_drop_event`.
- `ebpf/types.h` — `probe_params_t` carries where the context lives
  (`CTX_LOC_*`), plus `trace_decision_t`.
- `compiler/generate.go` — `resolveTraceIDSource` picks the context parameter
  and its location; appends the session throttler.
- `irgen` / `ir` — `CoordinatedSampling` config and its default rate.
- `loader` — publishes `session_throttler_idx` to the program.

Most of the eBPF code's shape comes from the verifier's 512-byte limit on the
combined stack of a call chain: working state lives in a per-CPU map, and the
two biggest helpers are noinline so their frames are not charged to the uprobe.
`ebpf/stackbudget` measures that chain from the compiled object and fails a test
when it gets too close.

## Not done yet

- Only amd64 has been exercised in-kernel.
- `WithCoordinatedSampling` is not wired to remote config, so the session rate
  is always the default.
- No test asserts the coordinated behavior end to end (whole chain or nothing,
  one snapshot per probe per request, independent requests, the negative-budget
  case). That needs a traced service that puts a real span in the context.
