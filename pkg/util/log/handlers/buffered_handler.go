// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"container/list"
	"context"
	"errors"
	"log/slog"
	"sync"
)

var _ slog.Handler = (*Buffered)(nil)

// Buffered is a slog handler that stores records in a buffer
type Buffered struct {
	buffer list.List
	lock   sync.Mutex
}

// NewBufferedHandler returns a handler that stores records in a buffer.
func NewBufferedHandler() *Buffered {
	return &Buffered{}
}

// Handle writes a record to the writer.
func (h *Buffered) Handle(ctx context.Context, r slog.Record) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.buffer.PushBack(msg{ctx, r})

	return nil
}

// Enabled returns true if the handler is enabled for the given level.
func (h *Buffered) Enabled(context.Context, slog.Level) bool {
	return true
}

// WithAttrs returns a new handler with the given attributes.
func (h *Buffered) WithAttrs(_attrs []slog.Attr) slog.Handler {
	panic("not implemented")
}

// WithGroup returns a new handler with the given group name.
func (h *Buffered) WithGroup(_name string) slog.Handler {
	panic("not implemented")
}

// Flush flushes the buffer to the handler.
func (h *Buffered) Flush(handler slog.Handler) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	var errs []error
	for e := h.buffer.Front(); e != nil; e = e.Next() {
		msg := e.Value.(msg)
		if err := handler.Handle(msg.ctx, msg.record); err != nil {
			errs = append(errs, err)
		}
	}

	h.buffer.Init() // clear the buffer

	return errors.Join(errs...)
}
