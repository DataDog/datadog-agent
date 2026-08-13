// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"log/slog"
)

var _ slog.Handler = (*level)(nil)

// level is a slog handler that filters logs based on a level.
type level struct {
	level        slog.Leveler
	innerHandler slog.Handler
	// packageLevel is level's packageLeveler view, resolved once at
	// construction time rather than on every Handle call: level.level never
	// changes identity after construction (only its own internal state may
	// mutate), so the type assertion's result cannot change either. Nil for a
	// plain slog.Leveler with no per-package support.
	packageLevel packageLeveler
}

// packageLeveler is implemented by Levelers that can additionally decide
// whether a specific call site is enabled, based on the package it belongs
// to (e.g. *types.Levels). Handle uses it, when available, to apply
// per-package overrides on top of the coarser Level() bound used by Enabled.
type packageLeveler interface {
	slog.Leveler
	EnabledForPC(pc uintptr, level slog.Level) bool
}

// NewLevel returns a handler that filters logs based on a level.
func NewLevel(lvl slog.Leveler, innerHandler slog.Handler) slog.Handler {
	pl, _ := lvl.(packageLeveler)
	return &level{level: lvl, innerHandler: innerHandler, packageLevel: pl}
}

// Enabled returns true if the handler is enabled for the given level. It
// never resolves a caller's package: for a Leveler with per-package rules,
// it reports whether level is enabled by any of them, so that callers can
// cheaply skip building a record for a level that's disabled everywhere.
func (h *level) Enabled(_ context.Context, level slog.Level) bool {
	return h.level.Level() <= level
}

// Handle writes a record to the innerHandler.
func (h *level) Handle(ctx context.Context, r slog.Record) error {
	// EnabledForPC's own decision already subsumes the coarse Enabled() bound
	// (they agree exactly when no per-package rules are configured, and
	// EnabledForPC is strictly more accurate otherwise), so it alone decides
	// here — this keeps the packageLeveler case down to a single config load,
	// on par with the plain-Leveler case below.
	if h.packageLevel != nil {
		if !h.packageLevel.EnabledForPC(r.PC, r.Level) {
			return nil
		}
	} else if !h.Enabled(ctx, r.Level) {
		return nil
	}

	return h.innerHandler.Handle(ctx, r)
}

// WithAttrs returns a new handler with the given attributes.
func (h *level) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &level{
		level:        h.level,
		innerHandler: h.innerHandler.WithAttrs(attrs),
		packageLevel: h.packageLevel,
	}
}

// WithGroup returns a new handler with the given group name.
func (h *level) WithGroup(name string) slog.Handler {
	return &level{
		level:        h.level,
		innerHandler: h.innerHandler.WithGroup(name),
		packageLevel: h.packageLevel,
	}
}
