# Go context.Context trace correlation

This document explains how dyninst captures dd-trace correlation IDs from
captured `context.Context` values and surfaces them on snapshot messages.

## Goal

When a probe captures a value that transitively contains a `context.Context`
(e.g. an `*http.Request` argument, or a bare `ctx context.Context` parameter),
dyninst walks the context's parent chain at probe time, looks for an active
dd-trace span, and emits the resulting trace_id / span_id / parent_id in two
places on the decoded snapshot:

- **At the message top level** as `dd.trace_id` / `dd.span_id` /
  `dd.parent_id` — the standard agent-side log-correlation tags.
- **Inline at the captured field** — replacing the normal interface chase, so
  a captured `*http.Request`'s `ctx` field renders as
  `{type: "context.Context", trace_id: …, span_id: …, parent_id: …}` rather
  than the concrete `cancelCtx` / `valueCtx` impl's struct contents.

When a probe captures multiple Contexts that each resolve to an active span,
the *first one in expression order* wins for the message-level tags. Each
Context still gets its own field-site rendering.

## Non-goals

- We do not render the concrete context-impl struct's fields. Once we emit
  the synthetic data item, those bytes are gone — accepted as a trade for
  the simpler design described below.
- We do not propagate trace context out of the probed binary. dyninst is
  read-only.

## Architecture

```
                            entry-eval phase                         chase phase
                            ────────────────                         ───────────
probe captures *http.Request
                ↓
      ┌──────────────────────────┐    ┌─────────────────────────────────────┐
      │ scratch_buf_serialize    │    │ chase loop dequeues                 │
      │ writes header+payload    │    │ (cancelCtx, ctx_addr)               │
      │ for *http.Request        │    │                                     │
      │                          │    │ scratch_buf_serialize copies        │
      │ enqueue_pc descends into │    │ sizeof(cancelCtx) bytes;            │
      │ struct fields, hits ctx  │    │ writes header                       │
      │                          │    │ {type=cancelCtx, length, addr}      │
      │ SM_OP_PROCESS_GO_INTERFACE│    │ sm->offset → payload start          │
      │ resolves (tab,data),      │    │                                     │
      │ enqueues                 │    │ dispatches to enqueue_pc,           │
      │ (cancelCtx, ctx_addr)    │    │ which is [INIT, HOP, RETURN]:       │
      └──────────────────────────┘    │                                     │
                                      │   INIT:                             │
                                      │     - rewrite header.type to        │
                                      │       trace_context_type_id         │
                                      │     - zero first 40 bytes of payload│
                                      │     - seed go_context_walk          │
                                      │                                     │
                                      │   HOP (self-jumps until done):      │
                                      │     - look up type_info for         │
                                      │       current link                  │
                                      │     - try sm_maybe_extract_ddtrace_ │
                                      │       span_from_value_ctx; if       │
                                      │       hit, write trace_context_t    │
                                      │       at data_item_offset, done=1   │
                                      │     - else advance via              │
                                      │       go_context_context_offset and │
                                      │       sm_resolve_go_interface_at;   │
                                      │       sm->pc -= 1 (re-enter HOP)    │
                                      │                                     │
                                      │   RETURN:                           │
                                      │     - return to chase preamble      │
                                      └─────────────────────────────────────┘
                                                       ↓
                                          synthetic data item shipped to userspace
                                          {type=trace_context_type_id,
                                           length=sizeof(cancelCtx),
                                           address=ctx_addr,
                                           payload[0:40] = trace_context_t}
                                                       ↓
                                          decoder ingests data items in arrival
                                          order; on each *ir.TraceContextType
                                          it parses the 40-byte payload and, if
                                          valid=1 and ce.traceContext is empty,
                                          populates it (first-valid-wins).
                                                       ↓
                                          interface field-site rendering:
                                          encodeInterface tries
                                          dataItems[(ifaceTypeID, addr)] first;
                                          on miss, tries
                                          dataItems[(traceContextTypeID, addr)];
                                          if found, renders trace context
                                          shape; else falls through to normal
                                          notCapturedReason: depth.
```

## Why two opcodes (INIT + HOP)?

The chain walk needs ~32 hops worst-case. Putting it all in C would be one
opcode dispatch (cheaper) but folds many sm_loop iterations of work into a
single verifier-evaluated function — limiting future expansion of the
per-hop logic. Per-hop opcode dispatch lets the BPF verifier amortize state
exploration across many small program flows, leaving headroom for richer
extraction (e.g. capturing the `value` field of a valueCtx alongside the
key check).

INIT is split out because hop 0 needs a different state than hops 1..31
(starting IR type known directly from `sm->di_0.type` rather than resolved
via `go_runtime_type`), and INIT also has a one-time job (rewrite header,
zero payload). Folding those into HOP would require a "first time?" sentinel
on every dispatch.

## Why chase-time rewrite, not field-time emission?

An interface field's inline `(tab, data)` header is only 16 bytes — too
small to embed a 40-byte `trace_context_t`. We could emit the trace context
as a separate enqueued data item from the field-eval site, but that requires
synthesizing a `go_runtime_type` that the decoder can recognize as
"synthetic trace context, not a real Go type." The chase-time rewrite is
simpler: every concrete context.Context implementation type already gets a
chase preamble and an `enqueue_pc` dispatch; we just override the
`enqueue_pc` to run the chain walk instead of struct-field descent. The
inline interface header keeps its real `(go_runtime_type, ctx_addr)` shape;
the decoder's address-keyed fallback finds the synthetic data item without
needing any synthetic-tab convention.

## Discovering context implementations

The Go compiler frequently elides runtime-type metadata for unexported
struct types only used through interfaces (cancelCtx, valueCtx, etc.).
Without those types in the IR catalog, the chain walk would have nothing
to look up at runtime.

irgen handles this by: (1) the gotype iteration scans for the special
type names listed in `pkg/dyninst/irgen/go_context.go:ddTraceGoContextTypes`
matching against both the short name (`context.cancelCtx`) and the
full-path-qualified name; (2) any matched type's DWARF offset is recorded
in `specialAdditionalTypeOffsets`; (3) when `needsGoContextSupport` is
true (via `analyzedProbesContainGoContext` OR a `context.Context` match in
the gotype iteration), all matched types — including `context.Context`
itself — are added to the type catalog via `addType` and registered as
exploration roots.

`context.Context` gets budget 2 (not 1) so that the unified expansion
dereferences both the interface (cost 1 → impl pointer) and the impl
pointer (cost 1 → impl struct), materializing the struct as a real
`StructureType` rather than a `pointeePlaceholderType` that would later
collapse to `UnresolvedPointeeType`.

## Identifying the active span

The dd-trace `internal.contextKey` (the key used by `tracer.ContextWithSpan`)
is a zero-byte unexported struct whose runtime type the Go compiler often
elides — there's no way to recognize it by type at BPF time. Instead,
`sm_maybe_extract_ddtrace_span_from_value_ctx` recognizes the active span
by its **value** type: it reads the valueCtx's value as an `any`, looks
up the IR type, and checks `ddtrace_span_kind != 0`
(`sm_extract_ddtrace_span` does this internally). If the value is a known
dd-trace span layout (V1 or V2), we extract; otherwise we keep walking.

## Wrapper IR types

Rather than carry chain-walk metadata as fields on every `StructureType` (via
the `GoContextAttributes` / `DDTraceAttributes` blobs once embedded in
`GoTypeAttributes`), irgen wraps the affected struct types in dedicated IR
types after the main type-graph build:

- `*ir.GoContextImplementationType` — wraps `*StructureType` for any concrete
  context.Context implementation (stdlib impls like `cancelCtx`, `valueCtx`,
  `timerCtx`, `stopCtx`, `afterFuncCtx`, `withoutCancelCtx`, `signalCtx`,
  `backgroundCtx`, `todoCtx`, `emptyCtx`, plus application types that satisfy
  the interface). Carries `GoContextAttributes` (the embedded Context offset,
  plus key/value offsets for valueCtx).

  A wrapped type only drives the chain walk when it is a *chain link*, i.e.
  `GoContextAttributes.HasChainData()` is true: it has an embedded parent
  Context or (for valueCtx) a key/value payload. An impl with none of these is
  not a chain link: it implements context.Context without holding a context of
  its own (e.g. a request type whose methods forward to the request's
  `Context()`, or a terminal root like `context.Background`), so there is no
  chain to walk. Such a type keeps the descriptive
  `GoContextImplementationType` label, but the compiler emits ordinary
  struct-field descent for it and the loader leaves `go_context_is_context`
  unset and does not floor `byte_len` — its capture is byte-for-byte identical
  to a plain struct, so its own fields are captured. Running it through the
  chain walk instead would rewrite its data item to a (dead-end, all-zero)
  trace context and discard the struct's fields.
- `*ir.DDTraceSpanType` — wraps the dd-trace `tracer.span` (v1) /
  `tracer.Span` (v2) struct type. Carries `DDTraceAttributes` (kind, span
  ID / trace ID / parent ID offsets, span context offsets).

The wrap happens in `annotateSpecialGoTypes` by overwriting the
`typeCatalog.typesByID[id]` entry. Compiler `addTypeHandler` recognizes the
wrappers and emits `[INIT, HOP, RETURN]` as the enqueue_pc for
`*GoContextImplementationType` and pointers to it; `*DDTraceSpanType`
behaves like its inner `*StructureType` for chase emission. This keeps
`GoTypeAttributes` slim (just `GoRuntimeType` + `GoKind`) on every other
IR type.

## Trade-offs

- **Buffer waste**: the chase preamble copies `sizeof(cancelCtx)` (typically
  ~80 bytes) before INIT overwrites the first 40 bytes. Trailing bytes are
  unread by the decoder. A few dozen wasted bytes per captured Context.
- **byte_len floor**: tiny chain-walked context impls can have `byte_len < 40`.
  The loader floors `byte_len` to 40 for any chain-walked
  `*ir.GoContextImplementationType` (i.e. `HasChainData()` true) and for
  pointer types whose pointee is one, so the payload reservation is always
  wide enough for INIT's 40-byte zero. Non-chain-link impls are not floored.
  See `pkg/dyninst/loader/serialize.go`.
- **No concrete impl rendering**: a captured Context's struct fields no
  longer appear in the snapshot. If a future user needs them, the rendering
  path can be re-introduced by emitting a sibling data item (synthetic +
  copied struct), but the design currently optimizes for the trace-id case.

## Files

- `pkg/dyninst/ir/types.go` — `TraceContextType` synthetic IR type;
  `GoContextImplementationType`, `DDTraceSpanType` wrappers around
  `*StructureType`.
- `pkg/dyninst/irgen/type_catalog.go` — allocates the synthetic type id.
- `pkg/dyninst/irgen/go_context.go` — replaces stdlib-context-impl and
  dd-trace-span entries in the type catalog with the wrapper types after
  type-graph construction.
- `pkg/dyninst/compiler/generate.go` — emits `[INIT, HOP, RETURN]` as the
  enqueue_pc subroutine for a chain-walked `*ir.GoContextImplementationType`
  (and pointer types whose pointee is one); an impl that is not a chain link
  (`HasChainData()` false) gets ordinary struct-field descent instead.
- `pkg/dyninst/loader/serialize.go` — floors `byte_len` to 40 for a
  chain-walked `*ir.GoContextImplementationType` and publishes the synthetic
  IR id as the BPF volatile-const `trace_context_type_id`.
- `pkg/dyninst/ebpf/stack_machine.h` — `SM_OP_GO_CONTEXT_CHAIN_INIT` and
  `SM_OP_GO_CONTEXT_CHAIN_HOP` opcode bodies; `sm_extract_ddtrace_span` /
  `sm_maybe_extract_ddtrace_span_from_value_ctx` helpers.
- `pkg/dyninst/decode/marshal.go` — first-valid-wins data-item ingestion
  populating `ce.traceContext`.
- `pkg/dyninst/decode/types.go` — `traceContextType` decoder type and the
  address-keyed fallback in `encodeInterface`.
