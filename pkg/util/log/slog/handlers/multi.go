// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"errors"
	"log/slog"
)

var _ slog.Handler = (*multi)(nil)

// multi is a handler that writes to multiple handlers.
//
// TODO: replace with built-in multi-handler in Go 1.26
type multi struct {
	handlers []slog.Handler
}

// NewMulti creates a new Handler that writes to multiple handlers.
func NewMulti(handlers ...slog.Handler) slog.Handler {
	if len(handlers) == 1 {
		return handlers[0]
	}
	return &multi{handlers: handlers}
}

// Handle writes a record to the handlers.
func (h *multi) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Enabled returns true if the handler is enabled for the given level.
func (h *multi) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// WithAttrs returns a new handler with the given attributes.
func (h *multi) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return NewMulti(handlers...)
}

// WithGroup returns a new handler with the given group name.
func (h *multi) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return NewMulti(handlers...)
}
