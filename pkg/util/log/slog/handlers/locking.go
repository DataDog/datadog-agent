// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"log/slog"
	"sync"
)

type locking struct {
	inner slog.Handler
	sync.Mutex
}

// NewLocking returns a new locking handler that wraps the given inner handler.
// Only the Handle method is synchronized, Enabled is not.
func NewLocking(inner slog.Handler) slog.Handler {
	return &locking{inner: inner}
}

func (s *locking) Handle(ctx context.Context, record slog.Record) error {
	s.Lock()
	defer s.Unlock()
	return s.inner.Handle(ctx, record)
}

// Enabled returns true if the inner handler is enabled for the given level.
func (s *locking) Enabled(ctx context.Context, level slog.Level) bool {
	return s.inner.Enabled(ctx, level)
}

// WithAttrs returns a new handler with the given attributes.
func (s *locking) WithAttrs([]slog.Attr) slog.Handler {
	panic("not implemented")
}

// WithGroup returns a new handler with the given group name.
func (s *locking) WithGroup(string) slog.Handler {
	panic("not implemented")
}
