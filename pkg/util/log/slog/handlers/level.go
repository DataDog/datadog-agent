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
}

// NewLevel returns a handler that filters logs based on a level.
func NewLevel(lvl slog.Leveler, innerHandler slog.Handler) slog.Handler {
	return &level{level: lvl, innerHandler: innerHandler}
}

// Enabled returns true if the handler is enabled for the given level.
func (h *level) Enabled(_ context.Context, level slog.Level) bool {
	return h.level.Level() <= level
}

// Handle writes a record to the innerHandler.
func (h *level) Handle(ctx context.Context, r slog.Record) error {
	if !h.Enabled(ctx, r.Level) {
		return nil
	}

	return h.innerHandler.Handle(ctx, r)
}

// WithAttrs returns a new handler with the given attributes.
func (h *level) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &level{
		level:        h.level,
		innerHandler: h.innerHandler.WithAttrs(attrs),
	}
}

// WithGroup returns a new handler with the given group name.
func (h *level) WithGroup(name string) slog.Handler {
	return &level{
		level:        h.level,
		innerHandler: h.innerHandler.WithGroup(name),
	}
}
