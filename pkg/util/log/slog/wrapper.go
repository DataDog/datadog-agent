// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package slog provides an slog implementation of the LoggerInterface interface.
package slog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

var _ types.LoggerInterface = (*Wrapper)(nil)

// baseStackDepth is the number of frames to skip when computing the caller PC
// for the per-level convenience methods (Trace, Debug, …) called directly on
// the Wrapper.  The chain is:
//
//	runtime.Callers          (0)
//	handle{Args,Format,Error}  (1)
//	Wrapper.{Trace,Debug,…}    (2)
//	caller                     (3)  ← captured
const baseStackDepth = 3

// Wrapper is a wrapper around the slog.Handler interface.
// It implements the LoggerInterface interface.
type Wrapper struct {
	handler slog.Handler
	closed  atomic.Bool
	flush   func()
	close   func()
}

// NewWrapper returns a new Wrapper implementing the LoggerInterface interface.
func NewWrapper(handler slog.Handler) types.LoggerInterface {
	return NewWrapperWithCloseAndFlush(handler, nil, nil)
}

// NewWrapperWithCloseAndFlush returns a new Wrapper implementing the LoggerInterface interface, with a flush and close function.
func NewWrapperWithCloseAndFlush(handler slog.Handler, flush func(), close func()) types.LoggerInterface {
	return &Wrapper{
		handler: handler,
		flush:   flush,
		close:   close,
	}
}

// Handler returns the slog.Handler used by the Wrapper.
func (w *Wrapper) Handler() slog.Handler {
	return w.handler
}

// handle creates and dispatches a slog.Record.  pc must already be the correct
// program counter of the original call site; no additional frame skipping is
// performed here.
func (w *Wrapper) handle(level types.LogLevel, pc uintptr, message string, attrs []slog.Attr) {
	r := slog.NewRecord(
		time.Now(),
		types.ToSlogLevel(level),
		message,
		pc,
	)
	if len(attrs) > 0 {
		r.AddAttrs(attrs...)
	}
	if err := w.handler.Handle(context.Background(), r); err != nil {
		fmt.Fprintf(os.Stderr, "log: wrapper internal error: %v\n", err)
	}
}

func (w *Wrapper) handleArgs(level types.LogLevel, v ...interface{}) {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		var pc [1]uintptr
		runtime.Callers(baseStackDepth, pc[:])
		w.handle(level, pc[0], renderArgs(v...), nil)
	}
}

func (w *Wrapper) handleFormat(level types.LogLevel, format string, params ...interface{}) {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		var pc [1]uintptr
		runtime.Callers(baseStackDepth, pc[:])
		w.handle(level, pc[0], renderFormat(format, params...), nil)
	}
}

func (w *Wrapper) handleError(level types.LogLevel, message string) error {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		var pc [1]uintptr
		runtime.Callers(baseStackDepth, pc[:])
		w.handle(level, pc[0], message, nil)
	}
	return errors.New(message)
}

// Log writes a pre-formatted, already-scrubbed message at the given level.
// pc is the program counter of the original call site, computed by the caller
// via runtime.Callers.  context is an optional flat slice of key-value pairs
// added as log attributes.
// Returns a non-nil error for Warn, Error, and Critical levels; nil otherwise.
func (w *Wrapper) Log(level types.LogLevel, pc uintptr, message string, ctx []interface{}) error {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		attrs := formatters.ToSlogAttrs(ctx)
		w.handle(level, pc, message, attrs)
	}
	if level >= types.WarnLvl {
		return errors.New(message)
	}
	return nil
}

func renderArgs(v ...interface{}) string {
	return fmt.Sprint(v...)
}

func renderFormat(format string, params ...interface{}) string {
	return fmt.Sprintf(format, params...)
}

// Trace formats message using the default formats for its operands
// and writes to log with level = Trace
func (w *Wrapper) Trace(v ...interface{}) {
	w.handleArgs(types.TraceLvl, v...)
}

// Tracef formats message according to format specifier
// and writes to log with level = Trace.
func (w *Wrapper) Tracef(format string, params ...interface{}) {
	w.handleFormat(types.TraceLvl, format, params...)
}

// Debug formats message using the default formats for its operands
// and writes to log with level = Debug
func (w *Wrapper) Debug(v ...interface{}) {
	w.handleArgs(types.DebugLvl, v...)
}

// Debugf formats message according to format specifier
// and writes to log with level = Debug.
func (w *Wrapper) Debugf(format string, params ...interface{}) {
	w.handleFormat(types.DebugLvl, format, params...)
}

// Info formats message using the default formats for its operands
// and writes to log with level = Info
func (w *Wrapper) Info(v ...interface{}) {
	w.handleArgs(types.InfoLvl, v...)
}

// Infof formats message according to format specifier
// and writes to log with level = Info.
func (w *Wrapper) Infof(format string, params ...interface{}) {
	w.handleFormat(types.InfoLvl, format, params...)
}

// Warn formats message using the default formats for its operands
// and writes to log with level = Warn
func (w *Wrapper) Warn(v ...interface{}) error {
	return w.handleError(types.WarnLvl, renderArgs(v...))
}

// Warnf formats message according to format specifier
// and writes to log with level = Warn.
func (w *Wrapper) Warnf(format string, params ...interface{}) error {
	return w.handleError(types.WarnLvl, renderFormat(format, params...))
}

// Error formats message using the default formats for its operands
// and writes to log with level = Error
func (w *Wrapper) Error(v ...interface{}) error {
	return w.handleError(types.ErrorLvl, renderArgs(v...))
}

// Errorf formats message according to format specifier
// and writes to log with level = Error.
func (w *Wrapper) Errorf(format string, params ...interface{}) error {
	return w.handleError(types.ErrorLvl, renderFormat(format, params...))
}

// Critical formats message using the default formats for its operands
// and writes to log with level = Critical
func (w *Wrapper) Critical(v ...interface{}) error {
	return w.handleError(types.CriticalLvl, renderArgs(v...))
}

// Criticalf formats message according to format specifier
// and writes to log with level = Critical.
func (w *Wrapper) Criticalf(format string, params ...interface{}) error {
	return w.handleError(types.CriticalLvl, renderFormat(format, params...))
}

// Close flushes all the messages in the logger and closes it. It cannot be used after this operation.
func (w *Wrapper) Close() {
	if !w.closed.CompareAndSwap(false, true) {
		// already closed, avoid calling the close function again
		return
	}
	if w.close != nil {
		w.close()
	}
}

// Flush flushes all the messages in the logger.
func (w *Wrapper) Flush() {
	if w.closed.Load() {
		return
	}

	if w.flush != nil {
		w.flush()
	}
}
