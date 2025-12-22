// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

var _ slog.Handler = (*Async)(nil)

// Async is a slog handler that asynchronously writes logs to another slog handler.
type Async struct {
	wg sync.WaitGroup

	// the condition is that either:
	// - there is something to flush
	// - there is something to write
	// - the handler is closed
	cond     *sync.Cond
	closed   bool
	msgQueue list.List
	flush    *flushMsg

	innerHandler slog.Handler
}

type flushMsg struct {
	queue list.List
	done  chan struct{}
}

type msg struct {
	ctx    context.Context
	record slog.Record
}

// NewAsync creates a new Async handler.
//
// The Async must be closed to avoid leaks.
func NewAsync(innerHandler slog.Handler) *Async {
	handler := &Async{
		innerHandler: innerHandler,
		cond:         sync.NewCond(&sync.Mutex{}),
	}

	handler.start()

	return handler
}

func (h *Async) writeList(queue list.List) {
	for e := queue.Front(); e != nil; e = e.Next() {
		msg := e.Value.(msg)
		err := h.innerHandler.Handle(msg.ctx, msg.record)
		if err != nil {
			fmt.Fprintf(os.Stderr, "log: async internal error: %v\n", err)
		}
	}
}

func (h *Async) start() {
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()

		h.cond.L.Lock()
		for {
			// if there is something to flush, do it
			if h.flush != nil {
				flush := h.flush
				h.flush = nil
				h.cond.L.Unlock()

				h.writeList(flush.queue)
				close(flush.done)

				h.cond.L.Lock()
				continue
			}

			// if there is something to write, do it
			if h.msgQueue.Len() > 0 {
				queue := h.msgQueue
				h.msgQueue = list.List{}
				h.cond.L.Unlock()

				h.writeList(queue)

				h.cond.L.Lock()
				continue
			}

			// if the handler is closed, exit
			if h.closed {
				h.cond.L.Unlock()
				break
			}

			h.cond.Wait()
		}
	}()
}

// Flush writes all messages in the queue to the inner handler.
func (h *Async) Flush() {
	h.cond.L.Lock()

	// if the handler is already closed, just wait for the goroutine to finish
	if h.closed {
		h.cond.L.Unlock()
		h.wg.Wait()
		return
	}

	// wait for the current flushes to finish
	for h.flush != nil {
		flushDone := h.flush.done
		h.cond.L.Unlock()
		<-flushDone
		h.cond.L.Lock()
	}

	// even if the queue is empty, we have to set the flush field and
	// wait for validation from the main loop
	// otherwise we could return while the main loop is still writing the messages

	queue := h.msgQueue
	h.msgQueue = list.List{}

	// set the new flush message
	done := make(chan struct{})
	h.flush = &flushMsg{queue, done}
	h.cond.Broadcast()
	h.cond.L.Unlock()

	// wait for the flush to finish
	<-done
}

// Handle writes a record to the handler.
func (h *Async) Handle(ctx context.Context, r slog.Record) error {
	h.cond.L.Lock()
	if h.closed {
		h.cond.L.Unlock()
		return nil
	}

	h.msgQueue.PushBack(msg{ctx, r})
	h.cond.Broadcast()
	h.cond.L.Unlock()

	return nil
}

// Enabled returns true if the handler is enabled for the given level.
func (h *Async) Enabled(ctx context.Context, level slog.Level) bool {
	return h.innerHandler.Enabled(ctx, level)
}

// Close closes the handler.
func (h *Async) Close() {
	h.cond.L.Lock()
	h.closed = true
	h.cond.Broadcast()
	h.cond.L.Unlock()

	h.wg.Wait() // wait for the goroutine to finish
}

// WithAttrs returns a new handler with the given attributes.
func (h *Async) WithAttrs(_attrs []slog.Attr) slog.Handler {
	panic("not implemented")
}

// WithGroup returns a new handler with the given group name.
func (h *Async) WithGroup(_name string) slog.Handler {
	panic("not implemented")
}
