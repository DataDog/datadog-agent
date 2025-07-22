// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log implements logging for the datadog agent.  It wraps seelog, and
// supports logging to multiple destinations, buffering messages logged before
// setup, and scrubbing secrets from log messages.
//
// # Compatibility
//
// This module is exported and can be used outside of the datadog-agent
// repository, but is not designed as a general-purpose logging system.  Its
// API may change incompatibly.
package log

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

type loggerPointer struct {
	atomic.Pointer[SlogWrapper]
}

var (
	// Logger is the main DatadogLogger
	logger    loggerPointer
	jmxLogger loggerPointer

	// for testing purposes
	scrubBytesFunc = scrubber.ScrubBytes
)

/*
*	Setup and initialization of the logger
 */

func init() {
	reset()
}

func reset() {
	makeDefaultLogger(&logger)
	makeDefaultLogger(&jmxLogger)
}

func makeDefaultLogger(loggerPtr *loggerPointer) {
	bufferedHandler := handlers.NewBufferedHandler()
	formatter := func(_ context.Context, r slog.Record) string {
		return fmt.Sprintf("%s: %s", LogLevel(r.Level), r.Message)
	}
	multiHandler := handlers.NewMultiHandler(
		bufferedHandler,
		handlers.NewLevelHandler(
			slog.Level(ErrorLvl),
			handlers.NewFormatHandler(formatter, os.Stderr),
		),
	)
	scrubberHandler := handlers.NewScrubberHandler(scrubber.DefaultScrubber, multiHandler)

	close := func() error {
		return bufferedHandler.Flush(loggerPtr.Load().Logger().Handler())
	}

	slogWrapper := NewSlogWrapperFixedLevel(slog.New(scrubberHandler), 1, TraceLvl, nil, close)
	loggerPtr.Store(slogWrapper)
}

// SetupLogger setup agent wide logger
func SetupLogger(i *SlogWrapper, level string) {
	lvl, err := ValidateLogLevel(level)
	if err != nil {
		i.SetLevel(lvl)
	}
	oldLogger := logger.Swap(i)
	oldLogger.Close()
}

func Flush() {
	if logger := logger.Load(); logger != nil {
		logger.Flush()
	}
	if logger := jmxLogger.Load(); logger != nil {
		logger.Flush()
	}
}

// GetLogLevel returns a seelog native representation of the current log level
func GetLogLevel() (LogLevel, error) {
	return logger.Load().LogLevel(), nil
}

// ShouldLog returns whether a given log level should be logged by the default logger
func ShouldLog(lvl LogLevel) bool {
	return logger.Load().Logger().Enabled(context.Background(), slog.Level(lvl))
}

// ValidateLogLevel validates the given log level and returns the corresponding Seelog log level.
// If the log level is "warning", it is converted to "warn" to handle a common gotcha when used with agent5.
// If the log level is not recognized, an error is returned.
func ValidateLogLevel(logLevel string) (LogLevel, error) {
	normLogLevel := strings.ToLower(logLevel)
	if normLogLevel == "warning" { // Common gotcha when used to agent5
		normLogLevel = "warn"
	}

	if lvl, found := LogLevelFromString(normLogLevel); found {
		return lvl, nil
	}
	return Off, fmt.Errorf("unknown log level: %s", normLogLevel)
}

// BuildLogEntry concatenates all inputs with spaces
func BuildLogEntry(v ...interface{}) string {
	var fmtBuffer bytes.Buffer

	for i := 0; i < len(v)-1; i++ {
		fmtBuffer.WriteString("%v ")
	}
	fmtBuffer.WriteString("%v")

	return fmt.Sprintf(fmtBuffer.String(), v...)
}

func scrubMessage(message string) string {
	msgScrubbed, err := scrubBytesFunc([]byte(message))
	if err == nil {
		return string(msgScrubbed)
	}
	return "[REDACTED] - failure to clean the message"
}

func formatError(v ...interface{}) error {
	msg := scrubMessage(fmt.Sprint(v...))
	return errors.New(msg)
}

func formatErrorc(message string, context ...interface{}) error {
	// Build a format string like this:
	// message (%s:%v, %s:%v, ... %s:%v)
	var fmtBuffer bytes.Buffer
	fmtBuffer.WriteString(message)
	if len(context) > 0 && len(context)%2 == 0 {
		fmtBuffer.WriteString(" (")
		for i := 0; i < len(context); i += 2 {
			fmtBuffer.WriteString("%s:%v")
			if i != len(context)-2 {
				fmtBuffer.WriteString(", ")
			}
		}
		fmtBuffer.WriteString(")")
	}

	msg := fmt.Sprintf(fmtBuffer.String(), context...)
	return errors.New(scrubMessage(msg))
}

// Trace logs at the trace level
func Trace(v ...interface{}) {
	logger.Load().Trace(v...)
}

// Tracef logs with format at the trace level
func Tracef(format string, params ...interface{}) {
	logger.Load().Tracef(format, params...)
}

// TracefStackDepth logs with format at the trace level and the current stack depth plus the given depth
func TracefStackDepth(depth int, format string, params ...interface{}) {
	logger.Load().Logf(TraceLvl, depth, nil, format, params...)
}

// TracecStackDepth logs at the trace level with context and the current stack depth plus the additional given one
func TracecStackDepth(message string, depth int, context ...interface{}) {
	logger.Load().Log(TraceLvl, depth, context, message)
}

// Tracec logs at the trace level with context
func Tracec(message string, context ...interface{}) {
	logger.Load().Log(TraceLvl, 0, context, message)
}

// TraceFunc calls and logs the result of 'logFunc' if and only if Trace (or more verbose) logs are enabled
func TraceFunc(logFunc func() string) {
	if logger := logger.Load(); logger != nil {
		if logger.Logger().Enabled(context.Background(), slog.Level(TraceLvl)) {
			logger.Log(TraceLvl, 0, nil, logFunc())
		}
	}
}

// Debug logs at the debug level
func Debug(v ...interface{}) {
	logger.Load().Debug(v...)
}

// Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	logger.Load().Debugf(format, params...)
}

// DebugfStackDepth logs with format at the debug level and the current stack depth plus the given depth
func DebugfStackDepth(depth int, format string, params ...interface{}) {
	logger.Load().Logf(DebugLvl, depth, nil, format, params...)
}

// DebugcStackDepth logs at the debug level with context and the current stack depth plus the additional given one
func DebugcStackDepth(message string, depth int, context ...interface{}) {
	logger.Load().Log(DebugLvl, depth, context, message)
}

// Debugc logs at the debug level with context
func Debugc(message string, context ...interface{}) {
	logger.Load().Log(DebugLvl, 0, context, message)
}

// DebugFunc calls and logs the result of 'logFunc' if and only if Debug (or more verbose) logs are enabled
func DebugFunc(logFunc func() string) {
	if logger := logger.Load(); logger != nil {
		if logger.Logger().Enabled(context.Background(), slog.Level(DebugLvl)) {
			logger.Log(DebugLvl, 0, nil, logFunc())
		}
	}
}

// Info logs at the info level
func Info(v ...interface{}) {
	logger.Load().Info(v...)
}

// Infof logs with format at the info level
func Infof(format string, params ...interface{}) {
	logger.Load().Infof(format, params...)
}

// InfofStackDepth logs with format at the info level and the current stack depth plus the given depth
func InfofStackDepth(depth int, format string, params ...interface{}) {
	logger.Load().Logf(InfoLvl, depth, nil, format, params...)
}

// InfocStackDepth logs at the info level with context and the current stack depth plus the additional given one
func InfocStackDepth(message string, depth int, context ...interface{}) {
	logger.Load().Log(InfoLvl, depth, context, message)
}

// Infoc logs at the info level with context
func Infoc(message string, context ...interface{}) {
	logger.Load().Log(InfoLvl, 0, context, message)
}

// InfoFunc calls and logs the result of 'logFunc' if and only if Info (or more verbose) logs are enabled
func InfoFunc(logFunc func() string) {
	if logger := logger.Load(); logger != nil {
		if logger.Logger().Enabled(context.Background(), slog.Level(InfoLvl)) {
			logger.Log(InfoLvl, 0, nil, logFunc())
		}
	}
}

// Warn logs at the warn level and returns an error containing the formated log message
func Warn(v ...interface{}) error {
	return logger.Load().Warn(v...)
}

// Warnf logs with format at the warn level and returns an error containing the formated log message
func Warnf(format string, params ...interface{}) error {
	return logger.Load().Warnf(format, params...)
}

// WarnfStackDepth logs with format at the warn level and the current stack depth plus the given depth
func WarnfStackDepth(depth int, format string, params ...interface{}) error {
	return logger.Load().LogfError(WarnLvl, depth, nil, format, params...)
}

// WarncStackDepth logs at the warn level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarncStackDepth(message string, depth int, context ...interface{}) error {
	return logger.Load().LogError(WarnLvl, depth, context, message)
}

// Warnc logs at the warn level with context and returns an error containing the formated log message
func Warnc(message string, context ...interface{}) error {
	return logger.Load().LogError(WarnLvl, 0, context, message)
}

// WarnFunc calls and logs the result of 'logFunc' if and only if Warn (or more verbose) logs are enabled
func WarnFunc(logFunc func() string) {
	if logger := logger.Load(); logger != nil {
		if logger.Logger().Enabled(context.Background(), slog.Level(WarnLvl)) {
			logger.Log(WarnLvl, 0, nil, logFunc())
		}
	}
}

// Error logs at the error level and returns an error containing the formated log message
func Error(v ...interface{}) error {
	return logger.Load().Error(v...)
}

// Errorf logs with format at the error level and returns an error containing the formated log message
func Errorf(format string, params ...interface{}) error {
	return logger.Load().Errorf(format, params...)
}

// ErrorfStackDepth logs with format at the error level and the current stack depth plus the given depth
func ErrorfStackDepth(depth int, format string, params ...interface{}) error {
	return logger.Load().LogfError(ErrorLvl, depth, nil, format, params...)
}

// ErrorcStackDepth logs at the error level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorcStackDepth(message string, depth int, context ...interface{}) error {
	return logger.Load().LogError(ErrorLvl, depth, context, message)
}

// Errorc logs at the error level with context and returns an error containing the formated log message
func Errorc(message string, context ...interface{}) error {
	return logger.Load().LogError(ErrorLvl, 0, context, message)
}

// ErrorFunc calls and logs the result of 'logFunc' if and only if Error (or more verbose) logs are enabled
func ErrorFunc(logFunc func() string) {
	if logger := logger.Load(); logger != nil {
		if logger.Logger().Enabled(context.Background(), slog.Level(ErrorLvl)) {
			logger.Log(ErrorLvl, 0, nil, logFunc())
		}
	}
}

// Critical logs at the critical level and returns an error containing the formated log message
func Critical(v ...interface{}) error {
	return logger.Load().Critical(v...)
}

// Criticalf logs with format at the critical level and returns an error containing the formated log message
func Criticalf(format string, params ...interface{}) error {
	return logger.Load().Criticalf(format, params...)
}

// CriticalfStackDepth logs with format at the critical level and the current stack depth plus the given depth
func CriticalfStackDepth(depth int, format string, params ...interface{}) error {
	return logger.Load().LogfError(CriticalLvl, depth, nil, format, params...)
}

// CriticalcStackDepth logs at the critical level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalcStackDepth(message string, depth int, context ...interface{}) error {
	return logger.Load().LogError(CriticalLvl, depth, context, message)
}

// Criticalc logs at the critical level with context and returns an error containing the formated log message
func Criticalc(message string, context ...interface{}) error {
	return logger.Load().LogError(CriticalLvl, 0, context, message)
}

// CriticalFunc calls and logs the result of 'logFunc' if and only if Critical (or more verbose) logs are enabled
func CriticalFunc(logFunc func() string) {
	if logger := logger.Load(); logger != nil {
		if logger.Logger().Enabled(context.Background(), slog.Level(CriticalLvl)) {
			logger.Log(CriticalLvl, 0, nil, logFunc())
		}
	}
}

// InfoStackDepth logs at the info level and the current stack depth plus the additional given one
func InfoStackDepth(depth int, v ...interface{}) {
	logger.Load().Log(InfoLvl, depth, nil, v...)
}

// WarnStackDepth logs at the warn level and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarnStackDepth(depth int, v ...interface{}) error {
	return logger.Load().LogError(WarnLvl, depth, nil, v...)
}

// DebugStackDepth logs at the debug level and the current stack depth plus the additional given one and returns an error containing the formated log message
func DebugStackDepth(depth int, v ...interface{}) {
	logger.Load().Log(DebugLvl, depth, nil, v...)
}

// TraceStackDepth logs at the trace level and the current stack depth plus the additional given one and returns an error containing the formated log message
func TraceStackDepth(depth int, v ...interface{}) {
	logger.Load().Log(TraceLvl, depth, nil, v...)
}

// ErrorStackDepth logs at the error level and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorStackDepth(depth int, v ...interface{}) error {
	return logger.Load().LogError(ErrorLvl, depth, nil, v...)
}

// CriticalStackDepth logs at the critical level and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalStackDepth(depth int, v ...interface{}) error {
	return logger.Load().LogError(CriticalLvl, depth, nil, v...)
}

/*
*	JMX Logger Section
 */

// JMXError Logs for JMX check
func JMXError(v ...interface{}) error {
	return jmxLogger.Load().Error(v...)
}

// JMXInfo Logs
func JMXInfo(v ...interface{}) {
	jmxLogger.Load().Info(v...)
}

// SetupJMXLogger setup JMXfetch specific logger
func SetupJMXLogger(i LoggerInterface, level string) {
	// TODO
	panic("not implemented")
}
