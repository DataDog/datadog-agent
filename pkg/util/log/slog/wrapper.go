// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package slog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/types"
)

const baseStackDepth = 4

// Wrapper is a wrapper around the slog.Handler interface.
// It implements the LoggerInterface interface.
type Wrapper struct {
	handler slog.Handler
	flush   func()
	close   func()

	attrs           []slog.Attr
	extraStackDepth int
}

func newWrapper(handler slog.Handler) *Wrapper {
	return newWrapperWithCloseAndFlush(handler, func() {}, func() {})
}

func newWrapperWithCloseAndFlush(handler slog.Handler, flush func(), close func()) *Wrapper {
	return &Wrapper{
		handler: handler,
		flush:   flush,
		close:   close,
	}
}

func (w *Wrapper) handleLazy(level types.LogLevel, message fmt.Stringer) {
	if w.handler.Enabled(context.Background(), slog.Level(level)) {
		w.handle(level, message.String())
	}
}

func (w *Wrapper) handleError(level types.LogLevel, message string) error {
	if w.handler.Enabled(context.Background(), slog.Level(level)) {
		w.handle(level, message)
	}
	return errors.New(message)
}

func (w *Wrapper) handle(level types.LogLevel, message string) {
	var pc [1]uintptr
	runtime.Callers(baseStackDepth+w.extraStackDepth, pc[:])
	r := slog.NewRecord(
		time.Now().UTC(),
		slog.Level(level),
		message,
		pc[0],
	)

	// we only set a context to perform a single log, so adding them directly on the record is fine
	// this can be optimized once we stop using seelog and can change the API
	if len(w.attrs) > 0 {
		r.AddAttrs(w.attrs...)
	}

	err := w.handler.Handle(context.Background(), r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "slog handler error: %v", err)
	}
}

type msgArgs struct {
	args []interface{}
}

func renderArgs(v ...interface{}) string {
	return fmt.Sprint(v...)
}

func (m *msgArgs) String() string {
	return renderArgs(m.args...)
}

type msgFormat struct {
	format string
	args   []interface{}
}

func renderFormat(format string, params ...interface{}) string {
	return fmt.Sprintf(format, params...)
}

func (m *msgFormat) String() string {
	return renderFormat(m.format, m.args...)
}

func newMsgArgs(v ...interface{}) fmt.Stringer {
	return &msgArgs{args: v}
}

func newMsgFormat(format string, v ...interface{}) fmt.Stringer {
	return &msgFormat{format: format, args: v}
}

// Trace formats message using the default formats for its operands
// and writes to log with level = Trace
func (w *Wrapper) Trace(v ...interface{}) {
	w.handleLazy(types.TraceLvl, newMsgArgs(v...))
}

// Tracef formats message according to format specifier
// and writes to log with level = Trace.
func (w *Wrapper) Tracef(format string, params ...interface{}) {
	w.handleLazy(types.TraceLvl, newMsgFormat(format, params...))
}

// Debug formats message using the default formats for its operands
// and writes to log with level = Debug
func (w *Wrapper) Debug(v ...interface{}) {
	w.handleLazy(types.DebugLvl, newMsgArgs(v...))
}

// Debugf formats message according to format specifier
// and writes to log with level = Debug.
func (w *Wrapper) Debugf(format string, params ...interface{}) {
	w.handleLazy(types.DebugLvl, newMsgFormat(format, params...))
}

// Info formats message using the default formats for its operands
// and writes to log with level = Info
func (w *Wrapper) Info(v ...interface{}) {
	w.handleLazy(types.InfoLvl, newMsgArgs(v...))
}

// Infof formats message according to format specifier
// and writes to log with level = Info.
func (w *Wrapper) Infof(format string, params ...interface{}) {
	w.handleLazy(types.InfoLvl, newMsgFormat(format, params...))
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
	w.close()
}

// Flush flushes all the messages in the logger.
func (w *Wrapper) Flush() {
	w.flush()
}

// SetAdditionalStackDepth sets the additional number of frames to skip by runtime.Caller
func (w *Wrapper) SetAdditionalStackDepth(depth int) error {
	w.extraStackDepth = depth
	return nil
}

// SetContext sets context which will be added to every log records
func (w *Wrapper) SetContext(context interface{}) {
	if context == nil {
		w.attrs = nil
		return
	}

	// See `extractContextString` in pkg/util/log/setup/log.go:
	// the context is a slice of interface{}, it contains an even number of elements,
	// and keys are strings.
	//
	// We can lift the restrictions and/or change the API later, but for now we want
	// the exact same behavior as we have with seelog

	ctx := context.([]interface{})
	var attrs []slog.Attr
	for i := 0; i < len(ctx); i += 2 {
		key, val := ctx[i], ctx[i+1]
		if keyStr, ok := key.(string); ok {
			attrs = append(attrs, slog.Attr{Key: keyStr, Value: slog.AnyValue(val)})
		}
	}
	w.attrs = attrs
}
