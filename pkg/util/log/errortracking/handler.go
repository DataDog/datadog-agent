// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"context"
	"log/slog"
	"runtime"
)

// stackSkipBase is the runtime.Callers skip parameter that drops the
// slog plumbing frames between the user's logger.Error(...) call site
// and our Handle. The value is locked by TestHandle_StackSkipBase in
// handler_test.go; do NOT change without re-running that test.
//
// Frame layout under runtime.Callers' semantics (skip=N starts at depth N):
//  0. runtime.Callers
//  1. Handler.Handle (this method)
//  2. (*slog.Logger).log
//  3. (*slog.Logger).Error / .Log
//  4. user code (the logger.Error(...) call site)
//
// skip=4 makes PCs[0] the user call site.
const stackSkipBase = 4

var _ slog.Handler = (*Handler)(nil)

// Handler is an slog.Handler that captures records at level >= Error and
// forwards them to the currently registered Submitter as an ErrorLog DTO.
//
// The handler holds no transport, no buffer and no goroutines. Each Handle
// call atomically loads the current Submitter via the load closure supplied
// at construction. When load returns nil the record is dropped silently;
// this is the steady state before the agenttelemetry component registers
// its Submitter during Fx startup, and again during test cleanup.
//
// The handler optionally also late-binds a per-PC Bouncer (see
// bouncer.go) via loadBouncer; when the closure returns non-nil and
// it suppresses the current PC, Handle returns without invoking the
// Submitter. The running count of suppressed dupes ships on the next
// non-suppressed sighting via ErrorLog.Count. Late-binding mirrors
// the Submitter pattern so the Bouncer's lifecycle (Fx start/stop) can
// be managed by the agenttelemetry component without restructuring
// the foundational logger build.
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
// The returned Handler has no Bouncer late-binder attached (every record
// is dispatched). Use WithBouncerLoader to enable per-PC dedup.
func NewHandler(load func() Submitter) *Handler {
	return &Handler{load: load}
}

// WithBouncerLoader returns a Handler that consults loadBouncer on
// every Handle to decide whether to suppress the current PC. loadBouncer
// MAY return nil at any time to disable dedup; the closure MUST be safe
// for concurrent use. Passing nil clears the late-binder.
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
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < slog.LevelError {
		return nil
	}
	submit := h.load()
	if submit == nil {
		return nil
	}

	out := ErrorLog{
		Time:  r.Time,
		PC:    r.PC,
		Count: 1,
	}
	// Bouncer check (when a loader is registered AND returns non-nil):
	// suppress duplicate PCs inside the window. The bouncer count rides
	// on the next non-suppressed sighting via ErrorLog.Count, so
	// operators can see the suppressed-duplicate count on the wire
	// without us shipping every occurrence.
	if h.loadBouncer != nil {
		if b := h.loadBouncer(); b != nil {
			suppressed, count, _ := b.Observe(r.PC, r.Time)
			if suppressed {
				return nil
			}
			out.Count = count
		}
	}
	// Capture a bounded multi-frame stack while the calling goroutine
	// is still on-stack — by the time the agenttelemetry flush
	// goroutine wakes up, the call chain that produced this record
	// would be gone. runtime.Callers is cheap (just walks the
	// stack-frame linked list and copies PC values); symbolization is
	// deferred to the sender.
	out.PCsLen = runtime.Callers(stackSkipBase, out.PCs[:])
	submit(out)
	return nil
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
