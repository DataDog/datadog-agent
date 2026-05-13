// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"context"
	"log/slog"
)

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
// The Submitter contract requires non-blocking submission (the consumer
// owns a bounded channel and flushes asynchronously), so Handle is
// non-blocking by construction. The handler is safe for concurrent use.
type Handler struct {
	load  func() Submitter
	attrs []slog.Attr
}

// NewHandler returns a Handler whose Handle method atomically loads the
// current Submitter via load on every record. load MUST be safe for
// concurrent use and MUST return nil to indicate "no submitter registered";
// nil records are dropped silently rather than panicking the logger chain.
func NewHandler(load func() Submitter) *Handler {
	return &Handler{load: load}
}

// Enabled reports whether the Handler will forward records at the given
// level. It returns true only when level >= slog.LevelError AND a Submitter
// is currently registered; an unregistered handler short-circuits the
// parent multi-handler so non-error formatting work is not wasted.
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
// Attrs (both WithAttrs-accumulated and record-level) and Message are
// captured on the DTO but emptied at the sender boundary
// (comp/core/agenttelemetry/impl/errortracking_sender.go::errorLogToLog).
// Every formatted message and every slog.Attr value is potentially
// user-controlled and therefore PII-suspect until template-aware capture
// lands; this PR ships PC-only telemetry. Keeping the fields on the DTO
// means template extraction can re-enable them in one place without
// re-plumbing the producer.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < slog.LevelError {
		return nil
	}
	submit := h.load()
	if submit == nil {
		return nil
	}

	submit(ErrorLog{
		Time:    r.Time,
		Level:   r.Level,
		Message: r.Message,
		PC:      r.PC,
	})
	return nil
}

// WithAttrs returns a Handler that prepends attrs to every captured record.
// Group nesting from prior WithGroup calls is intentionally not preserved -
// the wire format flattens groups anyway, and the previous nesting
// implementation was 40+ LOC of layered replay for no observable benefit.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &Handler{load: h.load, attrs: merged}
}

// WithGroup is a no-op. The wire payload does not distinguish nested
// groups from flat attrs, so preserving group structure here would only add
// complexity and allocation; subsequent WithAttrs calls accumulate at the
// top level.
func (h *Handler) WithGroup(_ string) slog.Handler {
	return h
}
