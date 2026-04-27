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
// submits them to an in-package Pipeline. It is intended to be plugged into
// the existing logger handler chain alongside the file/console handlers,
// AFTER the level filter so non-error records do not reach it.
//
// Handler is safe for concurrent use; the underlying Pipeline serializes
// dispatch internally.
type Handler struct {
	pipeline *Pipeline
	ops      []op
}

// op records a single WithAttrs or WithGroup operation in the order it
// was applied. The slog spec requires that subsequent WithAttrs after a
// WithGroup are nested inside that group; we therefore replay operations
// in order on Handle to rebuild the correct attribute structure.
type op struct {
	kind  opKind
	attrs []slog.Attr // valid when kind == opAttrs
	group string      // valid when kind == opGroup
}

type opKind int

const (
	opAttrs opKind = iota
	opGroup
)

// New returns a Handler that submits captured records to p.
func New(p *Pipeline) *Handler {
	return &Handler{pipeline: p}
}

// Enabled reports whether the Handler captures records at the given level.
// It captures level >= Error and nothing below.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

// Handle materializes the WithAttrs/WithGroup chain onto a copy of r and
// submits the result to the Pipeline. Always returns nil - errortracking
// must NEVER break the rest of the logger chain.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	// Defensive: slog calls Enabled before Handle, but a direct caller
	// might not. Drop non-error records silently.
	if r.Level < slog.LevelError {
		return nil
	}

	if len(h.ops) == 0 {
		h.pipeline.Submit(r)
		return nil
	}

	h.pipeline.Submit(h.applyOps(r))
	return nil
}

// applyOps returns a fresh Record with the recorded WithAttrs/WithGroup
// chain materialized. The original record's own attrs are placed at the
// innermost group level, matching slog.Handler spec semantics.
func (h *Handler) applyOps(r slog.Record) slog.Record {
	type layer struct {
		groupName string
		attrs     []slog.Attr
	}
	// layers[0] is the outermost (no enclosing group); each opGroup pushes
	// a new layer.
	layers := []layer{{}}
	for _, o := range h.ops {
		switch o.kind {
		case opGroup:
			layers = append(layers, layer{groupName: o.group})
		case opAttrs:
			last := &layers[len(layers)-1]
			last.attrs = append(last.attrs, o.attrs...)
		}
	}
	// The record's own attrs go at the innermost layer.
	r.Attrs(func(a slog.Attr) bool {
		last := &layers[len(layers)-1]
		last.attrs = append(last.attrs, a)
		return true
	})

	// Collapse innermost layers into nested slog.Group attrs.
	// Skip empty groups so we do not emit attrs like {grp: {}}.
	for i := len(layers) - 1; i > 0; i-- {
		inner := layers[i]
		if len(inner.attrs) == 0 {
			continue
		}
		args := make([]any, len(inner.attrs))
		for j, a := range inner.attrs {
			args[j] = a
		}
		grouped := slog.Group(inner.groupName, args...)
		layers[i-1].attrs = append(layers[i-1].attrs, grouped)
	}

	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	nr.AddAttrs(layers[0].attrs...)
	return nr
}

// WithAttrs returns a new Handler that prepends attrs to every captured
// record at the current group nesting level (per slog.Handler spec).
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.with(op{kind: opAttrs, attrs: attrs})
}

// WithGroup returns a new Handler that nests subsequent WithAttrs and the
// captured record's own attrs inside a group named name. WithGroup("") is
// a no-op per slog.Handler spec.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.with(op{kind: opGroup, group: name})
}

func (h *Handler) with(o op) *Handler {
	nh := &Handler{
		pipeline: h.pipeline,
		ops:      make([]op, len(h.ops), len(h.ops)+1),
	}
	copy(nh.ops, h.ops)
	nh.ops = append(nh.ops, o)
	return nh
}
