// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log implements the trace-agent logger.
package log

import (
	"sync"
)

var (
	mu     sync.RWMutex
	logger Logger = NoopLogger
)

// SetLogger sets l as the default Logger and returns the old logger.
func SetLogger(l Logger) Logger {
	mu.Lock()
	oldlogger := logger
	logger = l
	mu.Unlock()
	return oldlogger
}

// IsSet returns whether the logger has been set up.
func IsSet() bool {
	mu.Lock()
	defer mu.Unlock()
	return logger != NoopLogger
}

// Logger implements the core logger interface.
type Logger interface {
	Trace(v ...interface{})
	Tracef(format string, params ...interface{})
	Debug(v ...interface{})
	Debugf(format string, params ...interface{})
	Info(v ...interface{})
	Infof(format string, params ...interface{})
	Warn(v ...interface{}) error
	Warnf(format string, params ...interface{}) error
	Error(v ...interface{}) error
	Errorf(format string, params ...interface{}) error
	Critical(v ...interface{}) error
	Criticalf(format string, params ...interface{}) error
	Flush()
}

// Trace formats message using the default formats for its operands
// and writes to log with level = Trace
func Trace(v ...interface{}) {
	mu.RLock()
	logger.Trace(v...)
	mu.RUnlock()
}

// Tracef formats message according to format specifier
// and writes to log with level = Trace.
func Tracef(format string, params ...interface{}) {
	mu.RLock()
	logger.Tracef(format, params...)
	mu.RUnlock()
}

// Debug formats message using the default formats for its operands
// and writes to log with level = Debug
func Debug(v ...interface{}) {
	mu.RLock()
	logger.Debug(v...)
	mu.RUnlock()
}

// Debugf formats message according to format specifier
// and writes to log with level = Debug.
func Debugf(format string, params ...interface{}) {
	mu.RLock()
	logger.Debugf(format, params...)
	mu.RUnlock()
}

// Info formats message using the default formats for its operands
// and writes to log with level = Info
func Info(v ...interface{}) {
	mu.RLock()
	logger.Info(v...)
	mu.RUnlock()
}

// Infof formats message according to format specifier
// and writes to log with level = Info.
func Infof(format string, params ...interface{}) {
	mu.RLock()
	logger.Infof(format, params...)
	mu.RUnlock()
}

// Warn formats message using the default formats for its operands
// and writes to log with level = Warn
func Warn(v ...interface{}) {
	mu.RLock()
	logger.Warn(v...) //nolint:errcheck
	mu.RUnlock()
}

// Warnf formats message according to format specifier
// and writes to log with level = Warn.
func Warnf(format string, params ...interface{}) {
	mu.RLock()
	logger.Warnf(format, params...) //nolint:errcheck
	mu.RUnlock()
}

// Error formats message using the default formats for its operands
// and writes to log with level = Error
func Error(v ...interface{}) {
	mu.RLock()
	logger.Error(v...) //nolint:errcheck
	mu.RUnlock()
}

// Errorf formats message according to format specifier
// and writes to log with level = Error.
func Errorf(format string, params ...interface{}) {
	mu.RLock()
	logger.Errorf(format, params...) //nolint:errcheck
	mu.RUnlock()
}

// Critical formats message using the default formats for its operands
// and writes to log with level = Critical
func Critical(v ...interface{}) {
	mu.RLock()
	logger.Critical(v...) //nolint:errcheck
	mu.RUnlock()
}

// Criticalf formats message according to format specifier
// and writes to log with level = Critical.
func Criticalf(format string, params ...interface{}) {
	mu.RLock()
	logger.Criticalf(format, params...) //nolint:errcheck
	mu.RUnlock()
}

// Flush flushes all the messages in the logger.
func Flush() {
	mu.RLock()
	logger.Flush()
	mu.RUnlock()
}

// NoopLogger is a logger which has no effect upon calling.
var NoopLogger = noopLogger{}

type noopLogger struct{}

// Trace implements Logger.
func (noopLogger) Trace(_ ...interface{}) {}

// Tracef implements Logger.
func (noopLogger) Tracef(_ string, _ ...interface{}) {}

// Debug implements Logger.
func (noopLogger) Debug(_ ...interface{}) {}

// Debugf implements Logger.
func (noopLogger) Debugf(_ string, _ ...interface{}) {}

// Info implements Logger.
func (noopLogger) Info(_ ...interface{}) {}

// Infof implements Logger.
func (noopLogger) Infof(_ string, _ ...interface{}) {}

// Warn implements Logger.
func (noopLogger) Warn(_ ...interface{}) error { return nil }

// Warnf implements Logger.
func (noopLogger) Warnf(_ string, _ ...interface{}) error { return nil }

// Error implements Logger.
func (noopLogger) Error(_ ...interface{}) error { return nil }

// Errorf implements Logger.
func (noopLogger) Errorf(_ string, _ ...interface{}) error { return nil }

// Critical implements Logger.
func (noopLogger) Critical(_ ...interface{}) error { return nil }

// Criticalf implements Logger.
func (noopLogger) Criticalf(_ string, _ ...interface{}) error { return nil }

// Flush implements Logger.
func (noopLogger) Flush() {}
