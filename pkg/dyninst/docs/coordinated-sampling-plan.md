# Plan: Tier 1 Coordinated Sampling for Go Live Debugger

This plan implements Tier 1 of the RFC "Improving Correlation for Live Debugger
Snapshots" for the Go dynamic instrumentation module (`pkg/dyninst`).

## Goal

Replace the current independent per-probe sampling with a **coordinated decision
made once per trace execution**, enforced in eBPF *before* any capture work.
Every snapshot/capture probe that fires within the same trace shares one
EMIT/DROP outcome, so related snapshots arrive as complete chains instead of
fragments. Add a per-trace-per-probe cap (loop protection) and keep a global
session ceiling.

Scope is **Tier 1 only**: correlate on the `trace_id` that is already
retrievable when a `context.Context` is in scope; no new trace-context transport
mechanism. When no trace is in scope, fall back to today's independent per-probe
decision (RFC examples 3 and 5).

## Phase 0 conclusion: chosen approach (least resistance, reuse only)

Direction set by the owner: **reuse the existing, working `context.Context`
capture mechanism** to obtain trace_id; do NOT build a new eBPF extraction
feature and do NOT revive the dormant go-context value-extraction opcodes. In
the target sessions the span is top-of-chain, so the existing interface-capture
path already surfaces `traceID`. The implementation should be the simplest path
that reuses existing mechanisms, and only add the genuinely new bit that the
feature requires (the coordination decision itself).

Unresolved fork this creates (see decision log at end): the RFC and an earlier
decision place the EMIT/DROP decision in eBPF *before* capture (cheap drops).
But the existing, working trace_id read is a *capture-phase* mechanism. "Reuse
existing + simplest path" therefore points at **userspace coordination** (decide
from the already-captured trace_id in the decode/output pipeline, zero eBPF
changes), which trades away the RFC's "dropped probes cost nearly nothing"
guarantee. This tradeoff must be confirmed before Phases 1+.

## Re-baseline note (fresh main)

The plan was first drafted against an older tree. On fresh `main` the throttling
architecture has evolved and the design below is adjusted to it:

- Probes now have `Instances`, and events carry an optional `Condition`.
- Throttling has **modes** (`compiler.ThrottleMode` <-> `throttle_mode_t`):
  `THROTTLE_AT_START` (gate in `probe_run_with_cookie` before `probe_run`),
  `THROTTLE_AFTER_COND_CHECK` (gate inside `probe_run` after the condition
  passes), and `THROTTLE_NONE`. `computeThrottleMode` picks per event.
- Throttlers are shared per (probe, event-kind) via `throttlerByKind`.
- `probe_params` already carries `throttle_mode` and `throttler_idx`.

Impact on this design: the coordinated decision must slot into BOTH throttle
points (start and after-cond-check), replacing/也wrapping `should_throttle` with a
`sample_decision` that first tries the per-trace path, falling back to
`should_throttle`. trace_id must be read before whichever gate applies.

## Current state (verified)

- **Decision site:** `ebpf/event.c` -> `probe_run_with_cookie` calls
  `should_throttle(params->throttler_idx, start_ns)` *before* `probe_run`.
  Dropped probes already skip capture/stack-walk/emit. This is exactly where the
  coordinated decision belongs.
- **Throttler:** `ebpf/throttler.h` -- per-event
  `throttler_t{last_probe_run_ns, budget}` in `throttler_buf` array map,
  refreshed per `throttler_params.period_ns`. Generated one-per-event in
  `compiler/generate.go` from `probe.GetThrottleConfig()`.
- **Config:** `rcjson.Sampling{SnapshotsPerSecond}` -> snapshot probe = period
  1000ms, budget = `snapshotsPerSecond` (default 1), fully independent per probe.
- **Goroutine fields:** `read_g_fields` in `event.c` already dereferences `g`
  using DWARF offsets wired in `loader/loader.go`
  (`OFFSET_runtime_dot_g__{goid,m,stack}`, `runtime.m.curg`). This is the pattern
  we extend to read the in-scope trace context.
- **Header:** `output/framing_linux.go` carries `Goid`, `Stack_hash`,
  `Ktime_ns`. No trace_id today.
- **No trace_id retrieval exists in the Go DI eBPF path today** -- so the
  trace_id acquisition primitive is the one genuinely new build item; everything
  else is restructuring existing throttling.

## Sampling model (agreed)

- On probe fire, read the active `trace_id` (only possible when a
  `context.Context` is in scope; no new transport mechanism).
- **Per-trace decision map** keyed by `trace_id`. The **first event** for a
  trace_id is the entry/parent: it consults a **single session-global throttler**
  (today's budget/period logic) -> EMIT/DROP, and the outcome is stored in the
  map (via `BPF_NOEXIST` insert; concurrent first-firers race, losers inherit the
  winner's stored decision).
- **Every later event with the same trace_id inherits** the stored decision --
  no re-throttling.
- **Budget is consumed per emitted event** (entry + every inherited emit each
  decrement the global budget). Only the **entry decision** is *gated* by
  budget/period; **inherited EMIT events always emit** and still decrement, so the
  budget can go **negative**, which suppresses future traces' entry decisions
  until it refreshes. Chain completeness wins.
- **Per-trace-per-probe cap = 1**: the per-trace map entry carries a `probe_id`
  bitset (sized to `num_probe_params`); an inherited EMIT event only emits if its
  probe's bit is unset, then sets it. Bounds loop probes (RFC example 4) without a
  second map.
- **Fallback:** no `trace_id` in scope -> event runs today's per-probe throttler
  independently (RFC examples 3, 5 behavior preserved).

Per-trace decision map = `BPF_MAP_TYPE_LRU_HASH`, key `trace_id`, value
`{decision, generation_ts, emitted_probe_bitset}`.

## Design deltas vs. current code

- **`ebpf/throttler.h`**: keep `should_throttle` as the fallback path and reuse
  its budget/period mechanics for a **new session-global throttler instance**
  (single entry) that the entry decision + all emits draw from.
- **New `ebpf/coordinated_sample.h`**: `sample_decision(trace_id, present,
  probe_id)`:
  - `!present` -> return `should_throttle(per_probe_idx, start_ns)` (today's
    path).
  - `present` -> LRU lookup; if absent, run global throttler, `BPF_NOEXIST`
    insert `{decision, ts, bitset}`; if present, inherit. If EMIT and probe bit
    unset -> set bit, `__sync_fetch_and_sub` global budget (allow negative),
    emit; else drop.
- **`ebpf/event.c`**: in `probe_run_with_cookie`, replace the `should_throttle`
  gate with `sample_decision`, still before `probe_run` (dropped probes stay
  cheap). Add `read_trace_id()` next to `read_g_fields`.
- **`ebpf/program.h` / `types.h`**: declare the LRU decision map, the
  global-throttler entry, and cap/size constants.
- **`ebpf/framing.h` + `output/framing*.go`**: unchanged (trace_id not exposed on
  the wire -- Tier-1-only scope).
- **`compiler/generate.go`**: emit session-global throttler params + coordination
  config alongside existing per-probe throttlers (kept for fallback).
- **`loader/loader.go`**: create/size the LRU map + global throttler; set
  session-rate/cap constants; wire any new DWARF offsets needed by
  `read_trace_id` (same pattern as `OFFSET_runtime_dot_g__*`).
- **Config/IR**: session sampling rate field (Phase 1 decides new field vs.
  reinterpreting `snapshotsPerSecond`).

## Phases

- **Phase 0 -- trace_id read mechanism note.** Confirm exactly how to
  locate/read `trace_id` from the in-scope `context.Context` in eBPF (which
  value, offset path to the active span's trace_id, Go-version resilience, cost
  bound) + precise fallback rules. Only genuinely novel piece; de-risk first.
- **Phase 1 -- Config & IR plumbing.** Session rate + per-trace-per-probe cap
  into `ir`/`compiler.Program`; `generate.go` emits them.
- **Phase 2 -- eBPF trace_id read.** `read_trace_id()` + loader
  offsets/constants; returns trace_id + present flag.
- **Phase 3 -- eBPF coordinated decision.** `coordinated_sample.h`, global
  throttler, LRU decision map w/ bitset cap; swap the gate in
  `probe_run_with_cookie`.
- **Phase 4 -- Loader/userspace wiring.** Map creation/sizing + constants in
  `loader.go`.
- **Phase 5 -- Tests.** New `coordinated_sampling_test.go` + testprog fixture:
  nested/sequential/loop call sites threading a `context.Context`; assert (a)
  all-or-nothing per trace, (b) <=1 per probe per trace, (c) independent
  decisions across distinct traces, (d) per-event budget drawdown /
  negative-budget completeness, (e) fallback with no context.
- **Phase 6 -- Rollout guard.** Config flag, default off; document fallback +
  negative-budget semantics.

## Risks / open items

1. **Phase 0 trace_id read** -- the make-or-break. If it can't be done without a
   new mechanism, fallback-to-per-probe stays the default and we reconvene.
2. **LRU eviction mid-trace** -- a long/large trace evicted from the decision map
   re-runs the entry decision, risking a partial chain; size the map to make this
   rare, note as accepted edge.
3. **Session-global throttler contention** -- a single hot map entry under
   `__sync_fetch_and_sub`; the throttler's existing read-before-CAS pattern
   mitigates it.
4. **Cap bitset sizing** tied to `num_probe_params` per trace entry -- bounds
   memory per LRU slot.
5. Phase 1 config choice (new session-rate field vs. reinterpreting
   `snapshotsPerSecond`) still needs a decision when we get there.

## Phase 0 findings: trace_id acquisition mechanism (investigation complete)

Investigated how an active trace id can be read at probe time in eBPF for Go.

**There is no pre-existing, cheap mechanism that reads a trace id from a
lexically in-scope `context.Context`.** A `context.Context` is a runtime-typed
linked list of wrapper structs; the active span is stored under dd-trace-go's
private context key. Walking that chain from eBPF to recover the span (and thus
trace id) is a new, type- and version-fragile mechanism.

**The mechanism that does exist and is purpose-built for this correlation is the
tracer's goroutine pprof labels:**

- dd-trace-go (`internal/traceprof`) sets goroutine labels `"span id"` and
  `"local root span id"` via `pprof.SetGoroutineLabels` on span start
  (`ddtrace/tracer/tracer.go:applyPPROFLabels`), gated on code hotspots
  (`DD_PROFILING_CODE_HOTSPOTS_COLLECTION_ENABLED`).
- `"local root span id"` is constant across every span/goroutine in one trace
  within a service -> the ideal per-trace coordination key.
- Storage (Go 1.26): `runtime.g.labels (unsafe.Pointer) -> *labelMap{
  label.Set{ List []label.Label } }`, each `label.Label{Key, Value string}`,
  `Value` a decimal string. We already read `g` fields via DWARF offsets and
  `commonTypes.G` exposes the `labels` field offset, so eBPF can walk this.

**Caveats:**
1. Requires code hotspots enabled; otherwise no labels -> fall back to today's
   per-probe throttling.
2. `labelMap` layout is Go-version-dependent (older Go used `map[string]string`);
   the eBPF walk needs version handling.

### Reality check: context.Context trace_id capture already works today

Capturing a `context.Context` argument works today and surfaces the dd-trace-go
span fields (`traceID`, `spanID`, and `reparentID` -> `_dd.parent_id`). This is
NOT special trace_id logic; it is the **active** interface-processing path:

- `SM_OP_PROCESS_GO_INTERFACE = 17` and `SM_OP_PROCESS_GO_EMPTY_INTERFACE = 16`
  (defined + live in the `types.h` opcode enum).

When a probe captures `ctx`, the stack machine resolves the interface to the
concrete context type and chases pointers through the context chain deep enough
to reach the embedded `tracer.SpanContext`, whose fields (`traceID`, `spanID`,
`reparentID`) are then emitted as ordinary captured values. This is why a
repo-wide grep for `trace_id`/`span_id` in dyninst finds nothing, yet a live
session shows them: they are generic struct fields reached by generic capture.

**Consequence for Tier 1:** trace_id is definitively reachable in eBPF from an
in-scope `context.Context`. The open question is therefore not "can we read it"
but "can we read *just* trace_id cheaply and *before* capture," since the
capture above runs inside `sm_run` (after the throttle gate) via full interface
resolution + recursive pointer chasing. Tier 1 needs a targeted pre-capture
walk (ctx -> locate span -> read `traceID`) reusing the interface-chase
primitives, not the full recursive capture.

### Separately: dormant go-context *value extraction* scaffolding (different feature)

Unrelated to the trace_id capture above, there is also dormant scaffolding for a
distinct feature -- extracting specific values out of a context by key
(context-values / baggage), without capturing the whole chain. It lives in
`ebpf/stack_machine.h` as a set of opcodes and helpers:

- `SM_OP_PREPARE_GO_CONTEXT`, `SM_OP_TRAVERSE_GO_CONTEXT`,
  `SM_OP_CONCLUDE_GO_CONTEXT`
- `sm_resolve_go_context_value`, `sm_record_go_context_value`
- state fields `stack_machine_t.go_context_offset` /
  `go_context_capture_bitmask` (in `ebpf/context.h`, these ARE compiled in)
- type-info shape `go_context_impl{key_offset, value_offset, context_offset}`
  and `go_context_value` / `go_context_key`

This value-extraction feature is currently commented out (opcode enum stops at
`SM_OP_PREPARE_EVENT_ROOT = 22`, no `GO_CONTEXT` entries) with no Go-side
wiring; only `go_context_offset`/`go_context_capture_bitmask` in `context.h` are
compiled. It is NOT what produces the trace_id you see captured, and is not
required for Tier 1.

**Implication / decision for Tier 1:** since trace_id is already reachable from
an in-scope `context.Context` via the live interface-chase primitives, the build
is a narrow, purpose-built **pre-capture** extraction: locate the span in the
in-scope context and read its `traceID`, cheaply, before the throttle gate, then
feed it into the coordinated sampler. Open sub-questions: how to locate the
context argument/span reliably across call sites, and how to bound the cost of
the pre-capture walk. This gates Phases 1-5.

## Decision log

- Scope: Tier 1 only (coordinated per-trace sampling). Secondary gaps
  (runtime_id, generation token) deferred.
- Sampling model: entry event of a trace runs today's throttler; decision stored
  and inherited by later events in the same trace; budget consumed per emitted
  event; inherited EMIT events always emit (budget may go negative); per-trace
  per-probe cap = 1.
- Budget owner: single session-global throttler.
- trace_id source: reuse the existing, working context.Context interface-capture
  mechanism (top-of-chain span). No new extraction feature; no reviving the
  dormant go-context value-extraction opcodes.
- RESOLVED: decision site = eBPF, before capture (RFC-faithful cheap drops).
  Accepts new pre-capture eBPF trace_id read; still reuses existing primitives
  (interface deref, DWARF param locations, throttler); targets top-of-chain span
  with fallback to per-probe throttling when trace_id is not reachable.

## REVISED APPROACH: reorder the existing trace-context extraction (not a new pass)

Current main already ships a working context.Context -> trace_id/span_id/
parent_id extractor (`TraceContextType`; see `irgen/trace_context.md`). It runs
in the CHASE phase of `probe_run` (step 7 below), after both throttle gates,
because it is triggered as a side effect of pointer-chasing a *captured* context
value.

`probe_run` ordering: (1) header+goid, (2) pairing, (3) stack walk, (4)
`stack_machine_process_frame` = entry-eval (expressions + condition), (5)
condition_failed + `THROTTLE_AFTER_COND_CHECK` gate, (6) in_progress_calls
insert, (7) `stack_machine_chase_pointers` = chase (trace-context walk fires
here), (8) submit. `THROTTLE_AT_START` gates even earlier, in
`probe_run_with_cookie` before `probe_run`.

Tier 1 does NOT need a new extractor. It REORDERS: invoke the existing
chain-walk extraction at the TOP of `probe_run` (before the gate), keyed on a
known `context.Context` parameter, stash the resulting trace_id in an SM field,
then gate via `should_drop_event(trace_id, ...)`, then run the normal
`process_frame`/chase unchanged.

New work is small and bounded:
1. irgen: resolve a `context.Context` parameter's location for a coordinated
   probe even when it is not in the capture template (today a location is only
   guaranteed for captured expressions).
2. compiler: emit a tiny "sampling preamble" subroutine: load ctx from that
   location -> PROCESS_GO_INTERFACE -> existing chain walk -> store trace_id.
3. event.c: run the preamble first, feed trace_id into should_drop_event
   (commit 1 seam), then proceed as today.

Cost: the chain walk (interface resolve + up to ~32 hops) runs on every hit
before the gate, including future drops. Far cheaper than full capture; the
expensive value capture is still skipped on DROP. This is the price of
trace-coordinated pre-capture sampling.

## Concrete technical design (superseded pieces below kept for history)

### Reused, existing primitives (no reinvention)
- **Argument location:** `ir.Location`/`ir.Piece` (register / CFA / deref),
  already computed per expression at the probe PC and lowered by
  `compiler/generate.go` (`LocationOp` -> `ExprReadRegister` /
  `ExprDereferenceCfa` / `ExprDereferencePtr`). We reuse this to locate the
  in-scope `context.Context` argument.
- **Interface resolution:** `SM_OP_PROCESS_GO_INTERFACE` /
  `sm_resolve_go_interface` (live) to go from the `context.Context` iface to its
  concrete data pointer + runtime type.
- **Memory reads / g fields:** `bpf_probe_read_user`, `read_g_fields` pattern.
- **Throttler:** existing `should_throttle` budget/period mechanics, instantiated
  once as the session-global throttler.
- **DWARF offsets wiring:** the `loader/loader.go` `setVariable` +
  `FieldOffsetByName` pattern used for `runtime.g`/`runtime.m`.

### trace_id offset path (top-of-chain span)
`ctx` interface value `{itab/type, data}` -> `data` -> concrete top-of-chain
context struct -> reach `tracer.SpanContext` -> `SpanContext.traceID`
(`struct{ value [16]byte; hexEncoded string }`) -> use `value` (128-bit; low 64
bits are a sufficient sampling key) and optionally `spanID uint64`.

Offsets for the dd-trace-go `SpanContext.traceID(.value)` and `spanID` are
resolved from the target binary DWARF by type/field name in irgen (same way
`runtime.g` fields are resolved), gated on recognizing the concrete context type
that carries the span. When the concrete type is not a recognized
span-carrying context (not top-of-chain / no span), we do NOT walk further:
trace_id is treated as absent -> fallback.

### Phase 1 (Go: irgen + compiler)
- irgen: at each snapshot/capture probe, detect an in-scope parameter of
  interface type `context.Context`; resolve its `ir.Location` at the probe PC;
  resolve the concrete span-carrying context type + the `SpanContext.traceID`
  offset path from DWARF. Emit a compact "trace-id source" descriptor
  (location pieces + fixed offset chain) into the probe's IR, or mark trace_id
  unavailable.
- compiler/generate.go: lower the descriptor into a small pre-capture op
  sequence + populate `probe_params` with the trace-id read plan and the
  session sampling params.

### Phase 2 (eBPF: pre-capture read)
- `read_trace_id(regs, params, uint64* trace_id_lo, bool* present)` executed in
  `probe_run_with_cookie` BEFORE the gate: evaluate the location pieces to get
  the iface value, resolve the interface data pointer, follow the fixed offset
  chain, read `traceID.value` low 64 bits. Any failed read -> `present=false`.
- Keep it allocation-free and bounded (no full stack-machine run).

### Phase 3 (eBPF: coordinated decision) -- replaces the gate
- `sample_decision(trace_id_lo, present, probe_id, start_ns)`:
  - `!present` -> `should_throttle(per_probe_idx, start_ns)` (today's behavior).
  - `present` -> LRU lookup `decision_by_trace[trace_id_lo]`:
    - absent: run session-global `should_throttle`; `BPF_NOEXIST` insert
      `{decision, emitted_probe_bitset=0}`; losers of the race re-lookup and
      inherit.
    - present: inherit stored decision.
    - if EMIT and probe bit unset: set bit, `__sync_fetch_and_sub` global budget
      (allow negative), proceed to `probe_run`; else drop (cap or DROP).
- Maps: `decision_by_trace` = `BPF_MAP_TYPE_LRU_HASH` key `u64` value
  `{u8 decision; u64 bitset...}` (bitset sized to `num_probe_params`); global
  throttler = single-entry reuse of `throttler_t`.

### Phase 4/5/6
- loader: create/size `decision_by_trace`, set session-rate + cap constants,
  wire new `SpanContext` DWARF offsets; tests
  (`coordinated_sampling_test.go` + testprog threading `context.Context` through
  nested/sequential/loop sites); rollout flag default-off.

### Primary risks
- Recognizing the concrete span-carrying context type generically (vs. only the
  observed top-of-chain shape). Start with top-of-chain; fallback otherwise.
- dd-trace-go internal type/offset stability across tracer versions (mitigated
  by resolving from target DWARF at load time, not hardcoding).
- Cost/verifier budget of the pre-capture read on the hot path.

## Implementation status (this branch)

Implemented and always active (no enable flag): coordinated sampling is compiled
in unconditionally; a probe with no in-scope `context.Context` transparently
falls back to the per-probe throttler.

- eBPF: `coordinated_sample.h` (per-trace `decision_by_trace` LRU map,
  `coordinated_should_drop`, session-global throttler accounting, per-trace
  per-probe cap, `coord_extract_trace_id` pre-gate walk); `event.c`
  integration in `probe_run_with_cookie`; `program.h` `session_throttler_idx`;
  `types.h` `probe_params_t` ctx-location descriptor + `trace_decision_t`.
- Go: `ir.CoordinatedSampling` (+ `DefaultSessionSnapshotsPerSecond`) and
  `irgen.WithCoordinatedSampling`; compiler always appends the session
  throttler and resolves the `context.Context` location
  (`resolveTraceIDSource`); loader publishes `session_throttler_idx`;
  `types_linux.go` regenerated; unit tests for the resolver.

### Kernel-validated (BPF run in this VM via sudo)
- BPF verifier accepts the program (both `dyninst_event.o` and the
  `-debug` variant). Getting there fixed four real verifier/stack issues that
  compile-only checking missed:
  1. reading registers off the raw uprobe ctx pointer at a computed offset
     ("dereference of modified ctx ptr") — copy pt_regs into per-CPU scratch;
  2. an unbounded computed index into the per-trace bitset map value — index
     via a constant-offset switch;
  3. `struct pt_regs` on the BPF stack overflowing the 512-byte combined-stack
     limit — moved to per-CPU scratch;
  4. `coord_extract_trace_id` / `coordinated_should_drop` inlining their locals
     (incl. the 40-byte trace_decision_t and trace_context_t) into
     `probe_run_with_cookie`'s frame (which also inlines `probe_run`) and
     overflowing the stack — made both `noinline` and moved working state into
     per-CPU scratch; extraction is called once at the shallow top of
     `probe_run_with_cookie` and its result read from scratch at both gates.
- The full `pkg/dyninst` suite passes with coordination always-on:
  `TestThrottler`, `TestCircuitBreaker`, and the end-to-end `TestDyninst`
  across `simple`/`fault`/`panic_recover`/`sample` (the `sample`/`stack*`
  programs capture `context.Context`), plus drop-notification and fragment-cap
  tests. Confirms the program loads, capture still works, and the no-ctx
  fallback matches today's behavior.

### Still open (not validated here)
- arm64 (only amd64 exercised in this VM).
- Wiring `WithCoordinatedSampling` to real module/remote-config so a chosen
  session rate can be configured in a deployment (currently defaults to
  `DefaultSessionSnapshotsPerSecond`).
- Dedicated behavioral tests asserting the coordinated semantics directly
  (all-or-nothing per trace, <=1 per probe per trace, independent decisions
  across traces, per-event budget drawdown / negative-budget completeness) on
  a traced service that sets a dd-trace span in ctx.
- Note: the generated, gitignored `pkg/config/setup/{generated,all_settings,
  system_probe_settings}.go` had to be produced (bazel
  `//pkg/config/setup:codegen_settings`) before the test binary would link;
  they are not part of this change.
