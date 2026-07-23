// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"runtime"
)

// stackSearchBuf is the total PCs to allocate when scanning the current
// goroutine's stack for r.PC: MaxStackFrames frames we want to keep plus
// a generous headroom for the slog / pkg/util/log wrapper frames that sit
// between runtime.Callers and the user call site.
const stackSearchBuf = MaxStackFrames + 16

// stackPCsAttrKey is the slog attribute key used by SyncCapture to carry
// stack PCs captured in the emitting goroutine across an async handler
// boundary. Handler.Handle reads this attr before falling back to
// runtime.Callers so full-stack dedup is preserved even when the handler
// runs in an async worker goroutine.
const stackPCsAttrKey = "errortracking.pcs"

var _ slog.Handler = (*Handler)(nil)

// Handler is an slog.Handler that captures records at level >= Error and
// forwards them to the currently registered Submitter as an ErrorLog value.
//
// The handler holds no transport, no buffer and no goroutines. Each Handle
// call atomically loads the current Submitter via the load closure supplied
// at construction. When load returns nil the record is dropped silently;
// this is the steady state before the agenttelemetry component registers
// its Submitter during Fx startup, and again during test cleanup.
//
// The handler optionally also late-binds a per-stack Bouncer (see
// bouncer.go) via loadBouncer; when loadBouncer is set, the bouncer is
// consulted on every Handle and may suppress the record. The bouncer
// key is a FNV-1a hash of the captured stack PCs — two distinct stacks
// reaching the same terminal function are NOT collapsed into the same
// bouncer entry. The running count of suppressed dupes ships on the
// next non-suppressed sighting via ErrorLog.Count. Late-binding
// mirrors the Submitter pattern so the Bouncer's lifecycle (Fx
// start/stop) can be managed by the agenttelemetry component without
// restructuring the foundational logger build.
//
// The Submitter contract requires non-blocking submission (the consumer
// owns a bounded channel and flushes asynchronously), so Handle is
// non-blocking by construction. The handler is safe for concurrent use.
type Handler struct {
	load        func() Submitter
	loadBouncer func() *Bouncer
}

// NewHandler returns a Handler whose Handle method atomically loads the
// current Submitter via load on every record. load MUST be safe for
// concurrent use and MUST return nil to indicate "no submitter registered";
// nil records are dropped silently rather than panicking the logger chain.
//
// The returned Handler will DROP all records until a Bouncer is wired via
// WithBouncerLoader — a Bouncer is mandatory for submission.
func NewHandler(load func() Submitter) *Handler {
	return &Handler{load: load}
}

// WithBouncerLoader returns a Handler that consults loadBouncer on
// every Handle to decide whether to suppress the current record. The
// closure MUST be safe for concurrent use. Returning nil causes the
// record to be dropped — this is the safe behaviour when the bouncer
// is temporarily unavailable during Fx lifecycle transitions (startup /
// shutdown). Passing nil to WithBouncerLoader itself clears the
// late-binder; since a Bouncer is mandatory for submission, a nil
// loader causes all records to be dropped.
func (h *Handler) WithBouncerLoader(loadBouncer func() *Bouncer) *Handler {
	return &Handler{load: h.load, loadBouncer: loadBouncer}
}

// Enabled reports whether the Handler will forward records at the given
// level. It returns true only when level >= slog.LevelError AND a Submitter
// is currently registered; an unregistered handler short-circuits the
// parent multi-handler so non-error formatting work is not wasted.
//
// The "is a Submitter currently registered?" check is also the runtime
// gate for the agent_telemetry.errortracking.enabled config knob.
// installErrortrackingHandler (cmd/agent/subcommands/run) only calls
// pkg/util/log/setup.RegisterErrortrackingSubmitter when the gate is
// true — when the feature is off, the slot stays nil and Enabled
// returns false here, taking the steady-state path with no allocation.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	if level < slog.LevelError {
		return false
	}
	// h.load() is called again inside Handle. The slog.Handler interface
	// provides no mechanism to pass state between Enabled and Handle — the
	// framework calls them independently — so the second atomic load in
	// Handle is unavoidable. Both loads are cheap (atomic.Pointer.Load,
	// ~1–2 ns) and the nil-guard in Handle is the authoritative gate.
	return h.load() != nil
}

// Handle builds an ErrorLog from r and submits it. Records below Error are
// dropped (defensive: slog calls Enabled first, but direct callers might
// not). If no Submitter is registered the record is dropped silently.
// Handle always returns nil - errortracking must never break the rest of
// the logger chain.
//
// The handler captures only the wire-relevant fields (Time, PC, stack
// PCs, Count); message text and attrs are not captured because they are
// potentially user-controlled.
//
// Flow: level gate → submitter gate → capture stack PCs → (optional)
// bouncer check keyed by FNV-1a hash of the captured PCs → build
// ErrorLog → submit. When loadBouncer is set but returns nil the
// record is dropped (bouncer temporarily unavailable). The
// bouncer-key-is-a-hash-of-the-full-stack choice means two distinct
// stacks reaching the same terminal function each get their own dedup
// window.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < slog.LevelError {
		return nil
	}
	submit := h.load()
	if submit == nil {
		return nil
	}

	// Capture the full call stack anchored at r.PC.
	//
	// Preferred path: read PCs pre-captured by a SyncCapture wrapper that
	// ran synchronously in the emitting goroutine before any async
	// dispatch. Present when the wiring layer uses NewSyncCapture.
	//
	// Fallback path: call runtime.Callers here. Correct only when Handle
	// is called synchronously in the emitting goroutine (i.e. the handler
	// is NOT behind handlers.NewAsync or any other async boundary).
	var pcs [MaxStackFrames]uintptr
	var pcsLen int
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == stackPCsAttrKey {
			if captured, ok := a.Value.Any().([]uintptr); ok {
				pcsLen = copy(pcs[:], captured)
			}
			return false
		}
		return true
	})
	if pcsLen == 0 {
		var buf [stackSearchBuf]uintptr
		n := runtime.Callers(1, buf[:]) // skip runtime.Callers itself
		for i := 0; i < n; i++ {
			if buf[i] == r.PC {
				pcsLen = copy(pcs[:], buf[i:n])
				break
			}
		}
		if pcsLen == 0 && r.PC != 0 {
			// r.PC not found in the captured slice (async path or
			// synthetic record): store it alone so downstream consumers
			// always have at least a valid call-site frame.
			pcs[0] = r.PC
			pcsLen = 1
		}
	}

	var count uint32
	// Bouncer check: a Bouncer is mandatory for submission. Both a nil
	// loader (no bouncer registered at all) and a loader that returns nil
	// (temporarily unavailable during Fx startup/shutdown) cause the
	// record to be dropped — preventing an unrate-limited burst. The key
	// is a FNV-1a hash of the captured PCs so different stacks reaching
	// the same terminal function are NOT merged.
	var b *Bouncer
	if h.loadBouncer != nil {
		b = h.loadBouncer()
	}
	if b == nil {
		return nil
	}
	stackKey := hashPCs(pcs[:pcsLen])
	suppressed, c, _ := b.Observe(stackKey, r.Time)
	if suppressed {
		return nil
	}
	count = c

	out := ErrorLog{
		Time:      r.Time,
		PC:        r.PC,
		PCs:       pcs,
		PCsLen:    pcsLen,
		Count:     count,
		ErrorKind: errorKindFromRecord(r),
	}
	submit(out)
	return nil
}

// errorKindFromRecord returns the reflect type name of the first error-typed
// slog attribute in r (e.g. "*net.OpError"), or "" when none is present.
// The type name is code-determined, not user-controlled, so it is safe to
// ship on the wire unlike the error message itself.
func errorKindFromRecord(r slog.Record) string {
	var kind string
	r.Attrs(func(a slog.Attr) bool {
		v := a.Value.Resolve()
		if v.Kind() != slog.KindAny {
			return true
		}
		if err, ok := v.Any().(error); ok {
			kind = fmt.Sprintf("%T", err)
			return false // stop at first error attr
		}
		return true
	})
	return kind
}

// hashPCs returns a 64-bit FNV-1a hash of the captured stack PCs.
// The hash is the bouncer key — two records reaching the same terminal
// function from different call stacks produce different hashes and are
// NOT deduped together.
func hashPCs(pcs []uintptr) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, pc := range pcs {
		for i := range buf {
			buf[i] = byte(pc >> (8 * i))
		}
		h.Write(buf[:])
	}
	return h.Sum64()
}

// SyncCapture wraps any slog.Handler and pre-captures the goroutine's full
// call stack in the emitting goroutine before forwarding the record to the
// inner handler. This solves the async-boundary problem: when the inner
// handler is dispatched by an async worker goroutine, runtime.Callers can
// no longer see the original caller's frames. SyncCapture must be placed in
// the synchronous layer of the logger chain (before handlers.NewAsync or any
// other async wrapper) so that Handle is called in the same goroutine that
// emitted the log record.
//
// Handler.Handle reads the pre-captured PCs from the stackPCsAttrKey slog
// attribute added by SyncCapture.Handle, bypassing its own runtime.Callers
// call when the attr is present.
type SyncCapture struct {
	inner slog.Handler
}

var _ slog.Handler = (*SyncCapture)(nil)

// NewSyncCapture returns a SyncCapture that wraps inner. Install the returned
// handler in the synchronous layer of the logger chain so it is always called
// from the emitting goroutine.
func NewSyncCapture(inner slog.Handler) *SyncCapture {
	return &SyncCapture{inner: inner}
}

// Enabled delegates to the inner handler.
func (s *SyncCapture) Enabled(ctx context.Context, level slog.Level) bool {
	return s.inner.Enabled(ctx, level)
}

// Handle captures the current goroutine's call stack anchored at r.PC,
// attaches the PCs as a stackPCsAttrKey slog attribute, then forwards to
// the inner handler. Must be called from the goroutine that emitted r.
func (s *SyncCapture) Handle(ctx context.Context, r slog.Record) error {
	if r.Level < slog.LevelError {
		return s.inner.Handle(ctx, r)
	}
	var buf [stackSearchBuf]uintptr
	n := runtime.Callers(1, buf[:])
	for i := 0; i < n; i++ {
		if buf[i] == r.PC {
			pcs := make([]uintptr, n-i)
			copy(pcs, buf[i:n])
			r.AddAttrs(slog.Any(stackPCsAttrKey, pcs))
			break
		}
	}
	return s.inner.Handle(ctx, r)
}

// WithAttrs returns a SyncCapture wrapping the inner handler's WithAttrs result.
func (s *SyncCapture) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SyncCapture{inner: s.inner.WithAttrs(attrs)}
}

// WithGroup returns a SyncCapture wrapping the inner handler's WithGroup result.
func (s *SyncCapture) WithGroup(name string) slog.Handler {
	return &SyncCapture{inner: s.inner.WithGroup(name)}
}

// WithAttrs is a required slog.Handler interface method. Attrs are not
// shipped to the wire; this method is a required interface no-op.
func (h *Handler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

// WithGroup returns a new Handler instance with the same Submitter
// loader and Bouncer loader. The group name is intentionally discarded —
// the wire payload is flat and does not distinguish nested groups from
// top-level attrs, and we don't ship attrs to the wire anyway. Returning
// a NEW instance (rather than the receiver) matches the canonical
// shape of pkg/util/log/slog/handlers/multi.go::WithGroup and async.go::
// WithGroup; a no-op-receiver pattern can subtly break parent
// multi-handlers that expect each child to materialize a fresh
// instance per group context.
func (h *Handler) WithGroup(_ string) slog.Handler {
	return &Handler{load: h.load, loadBouncer: h.loadBouncer}
}
