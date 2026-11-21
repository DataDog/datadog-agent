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

const baseStackDepth = 4

// logStackBaseDepth is the number of stack frames runtime.Callers skips to
// reach the caller of Log/Logf: runtime.Callers itself, record, logMessage (or
// logMessagef) and Log (or Logf). The extraDepth argument of Log/Logf counts
// the frames to skip below that point.
const logStackBaseDepth = 4

// Wrapper is a wrapper around the slog.Handler interface.
// It implements the LoggerInterface interface.
type Wrapper struct {
	handler slog.Handler
	closed  atomic.Bool
	flush   func()
	close   func()

	attrs           atomic.Pointer[[]slog.Attr]
	extraStackDepth atomic.Int32
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

func (w *Wrapper) handleArgs(level types.LogLevel, v ...interface{}) {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		// rendering is only done if the level is enabled
		w.handle(level, renderArgs(v...))
	}
}

func (w *Wrapper) handleFormat(level types.LogLevel, format string, params ...interface{}) {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		// rendering is only done if the level is enabled
		w.handle(level, renderFormat(format, params...))
	}
}

func (w *Wrapper) handleError(level types.LogLevel, message string) error {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		w.handle(level, message)
	}
	return errors.New(message)
}

func (w *Wrapper) handle(level types.LogLevel, message string) {
	var pc [1]uintptr
	runtime.Callers(baseStackDepth+int(w.extraStackDepth.Load()), pc[:])
	r := slog.NewRecord(
		time.Now(),
		types.ToSlogLevel(level),
		message,
		pc[0],
	)

	// we only set a context to perform a single log, so adding them directly on the record is fine
	// this can be optimized once we stop using seelog and can change the API
	attrs := w.attrs.Load()
	if attrs != nil && len(*attrs) > 0 {
		r.AddAttrs(*attrs...)
	}

	err := w.handler.Handle(context.Background(), r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: wrapper internal error: %v\n", err)
	}
}

func renderArgs(v ...interface{}) string {
	return fmt.Sprint(v...)
}

func renderFormat(format string, params ...interface{}) string {
	return fmt.Sprintf(format, params...)
}

// Log writes the given message to the log with the given level, skipping
// extraDepth call frames below its own caller to compute the origin of the
// entry, and attaching the context key/value pairs to the record. Unlike the
// per-level methods, Log ignores the state set by SetContext.
//
// It returns an error containing the message for the warn, error and critical
// levels, and nil for the trace, debug and info levels.
func (w *Wrapper) Log(level types.LogLevel, message string, extraDepth int, context ...interface{}) error {
	return w.logMessage(level, message, extraDepth, context)
}

func (w *Wrapper) logMessage(level types.LogLevel, message string, extraDepth int, contextPairs []interface{}) error {
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		w.record(level, message, extraDepth, formatters.ToSlogAttrs(contextPairs))
	}
	if level >= types.WarnLvl {
		return errors.New(message)
	}
	return nil
}

// Logf formats message according to format specifier and writes it to log
// with the given level, skipping extraDepth call frames below its own caller
// to compute the origin of the entry.
//
// It returns an error containing the formatted message for the warn, error and
// critical levels, and nil for the trace, debug and info levels.
func (w *Wrapper) Logf(level types.LogLevel, format string, extraDepth int, params ...interface{}) error {
	return w.logMessagef(level, format, extraDepth, params)
}

func (w *Wrapper) logMessagef(level types.LogLevel, format string, extraDepth int, params []interface{}) error {
	// warn and above always render the message to return it as an error, like
	// the Warnf & co methods do
	alwaysRender := level >= types.WarnLvl
	message := ""
	if alwaysRender {
		message = renderFormat(format, params...)
	}
	if !w.closed.Load() && w.handler.Enabled(context.Background(), types.ToSlogLevel(level)) {
		if !alwaysRender {
			message = renderFormat(format, params...)
		}
		w.record(level, message, extraDepth, nil)
	}
	if alwaysRender {
		return errors.New(message)
	}
	return nil
}

// record writes one log record, resolving the origin of the entry
// logStackBaseDepth+extraDepth frames up the call stack, with the given
// attributes attached to the record.
func (w *Wrapper) record(level types.LogLevel, message string, extraDepth int, attrs []slog.Attr) {
	var pc [1]uintptr
	runtime.Callers(logStackBaseDepth+extraDepth, pc[:])
	r := slog.NewRecord(
		time.Now(),
		types.ToSlogLevel(level),
		message,
		pc[0],
	)

	if len(attrs) > 0 {
		r.AddAttrs(attrs...)
	}

	err := w.handler.Handle(context.Background(), r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: wrapper internal error: %v\n", err)
	}
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

// SetAdditionalStackDepth sets the additional number of frames to skip by runtime.Caller
func (w *Wrapper) SetAdditionalStackDepth(depth int) error {
	if w.closed.Load() {
		return nil
	}

	w.extraStackDepth.Store(int32(depth))
	return nil
}

// SetContext sets context which will be added to every log records
func (w *Wrapper) SetContext(context interface{}) {
	if w.closed.Load() {
		return
	}

	attrs := formatters.ToSlogAttrs(context)
	w.attrs.Store(&attrs)
}
