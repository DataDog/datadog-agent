// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log implements logging for the datadog agent. It buffers messages
// logged before setup.
//
// # Compatibility
//
// This module is exported and can be used outside of the datadog-agent
// repository, but is not designed as a general-purpose logging system.  Its
// API may change incompatibly.
package log

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

type loggerPointer struct {
	atomic.Pointer[DatadogLogger]
}

var (
	// Logger is the main DatadogLogger
	logger loggerPointer

	// This buffer holds log lines sent to the logger before its
	// initialization. Even if initializing the logger is one of the first
	// things the agent does, we still: load the conf, resolve secrets inside,
	// compute the final proxy settings, ...
	//
	// This buffer should be very short lived.
	logsBuffer  = []func(){}
	bufferMutex sync.Mutex

	// for testing purposes
	scrubBytesFunc = scrubber.ScrubBytes
)

// DatadogLogger wrapper structure for seelog
type DatadogLogger struct {
	inner LoggerInterface
	rules *types.RulesSync
	l     sync.RWMutex
}

/*
*	Setup and initialization of the logger
 */

// SetupLogger setup agent wide logger with a simple log level (e.g.
// "debug"), in particular it doesn't accept per package rules.
func SetupLogger(i LoggerInterface, level string) {
	logLevel, err := ValidateLogLevel(level)
	if err != nil {
		logLevel = InfoLvl
	}

	SetupLoggerWithLevelRules(i, types.NewRulesSyncFromLevel(logLevel))
}

// SetupLoggerWithLevelRules setup agent wide logger with support for dynamic
// log level changes.
func SetupLoggerWithLevelRules(i LoggerInterface, levelRules *types.RulesSync) {
	setupCommonLogger(i, levelRules)

	// Flush the log entries logged before initialization now that the logger is initialized
	bufferMutex.Lock()
	defer bufferMutex.Unlock()
	for _, logLine := range logsBuffer {
		logLine()
	}
	logsBuffer = []func(){}
}

func setupCommonLogger(i LoggerInterface, levelRules *types.RulesSync) {
	l := &DatadogLogger{
		inner: i,
		rules: levelRules,
	}

	oldLogger := logger.Load()
	logger.Store(l)

	if oldLogger != nil {
		oldLogger.l.Lock()
		if oldLogger.inner != nil {
			oldLogger.inner.Close()
		}
		oldLogger.l.Unlock()
	}
}

func addLogToBuffer(bufferFunc func(callerPos string), extraDepth int) {
	var callerPos string
	// skip this function's frame, then the internal helper, the exported log
	// function and any extra frames below them (see helperStackDepth)
	if _, file, line, ok := runtime.Caller(extraDepth + 1); ok {
		callerPos = fmt.Sprintf("%s:%d ", file, line)
	}

	bufferMutex.Lock()
	defer bufferMutex.Unlock()

	logsBuffer = append(logsBuffer, func() { bufferFunc(callerPos) })
}

/*
*	Operation on the **logger level**
 */

// ChangeLogLevel changes the current log level, valid levels are trace, debug,
// info, warn, error, critical and off.
func ChangeLogLevel(level LogLevel) error {
	return logger.changeLogLevelRules(types.NewLevelRules(level))
}

// ChangeLogLevelRules changes the current log level rules.
func ChangeLogLevelRules(levelRules *types.LevelRules) error {
	return logger.changeLogLevelRules(levelRules)
}

func (sw *loggerPointer) changeLogLevelRules(levelRules *types.LevelRules) error {
	l := sw.Load()
	if l == nil {
		return errors.New("cannot change loglevel: logger not initialized")
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		return errors.New("cannot change loglevel: logger is initialized however logger.inner is nil")
	}

	l.rules.Store(levelRules)

	return nil
}

// GetLogLevel returns a seelog native representation of the current log level
func GetLogLevel() (LogLevel, error) {
	return logger.getLogLevel()
}
func (sw *loggerPointer) getLogLevel() (LogLevel, error) {
	l := sw.Load()
	if l == nil {
		return InfoLvl, errors.New("cannot get loglevel: logger not initialized")
	}

	l.l.RLock()
	defer l.l.RUnlock()

	if l.inner == nil {
		return InfoLvl, errors.New("cannot get loglevel: logger not initialized")
	}

	return l.rules.Load().DefaultLevel(), nil
}

// GetLogLevelSpec returns the specification string of the current log level
// rules, e.g. "error,some/pkg/...=debug", or the plain level name when a
// single level is configured.
func GetLogLevelSpec() (string, error) {
	return logger.getLogLevelSpec()
}

func (sw *loggerPointer) getLogLevelSpec() (string, error) {
	l := sw.Load()
	if l == nil {
		return "", errors.New("cannot get loglevel: logger not initialized")
	}

	l.l.RLock()
	defer l.l.RUnlock()

	if l.inner == nil {
		return "", errors.New("cannot get loglevel: logger not initialized")
	}

	return l.rules.Load().Spec(), nil
}

// ShouldLog returns whether a given log level should be logged by the default logger.
// When the logger is not initialized yet, the default info level is assumed: info and
// above pass the check (their entries are buffered by the log helpers until a logger
// is set up), while trace and debug are dropped.
func ShouldLog(lvl LogLevel) bool {
	// The lock stay in the exported function due to the use of `shouldLog` in function that already hold the lock
	l := logger.Load()
	if l != nil {
		l.l.RLock()
		defer l.l.RUnlock()
		return l.shouldLog(lvl)
	}
	return lvl >= InfoLvl
}

// This function should be called with `sw.l` held
func (sw *DatadogLogger) shouldLog(level LogLevel) bool {
	return level >= sw.rules.Load().MinLevel()
}

// ValidateLogLevel validates the given log level and returns the
// corresponding LogLevel.
func ValidateLogLevel(logLevel string) (LogLevel, error) {
	return types.ValidateLogLevel(logLevel)
}

/*
*	Operation on the **logger**
 */

// Flush flushes the underlying inner log
func Flush() {
	logger.flush()
}

func (sw *loggerPointer) flush() {
	l := sw.Load()
	if l == nil {
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner != nil {
		l.inner.Flush()
	}
}

/*
*	log functions
 */

// helperStackDepth is the number of call frames between the internal helpers
// below (log, logFormat, logContext, ...) and the code calling the exported
// log functions: the exported function itself plus the internal helper. It is
// passed to LoggerInterface.Log/Logf, which skip that many frames below their
// own caller to attribute log entries to the caller of the exported function.
//
// The exported functions add to it for every additional frame they introduce
// below that caller: the *StackDepth variants skip depth-1 more frames (the
// c-variants depth, their depth being counted from their own caller), and
// helpers such as the comp/core/log wrapper add one frame per nesting level.
const helperStackDepth = 2

// log logs a message at the given level, using bufferFunc (if logging is not
// yet set up) to buffer a closure emitting the entry, and concatenating v...
// with spaces like the exported functions do.
func log(logLevel LogLevel, bufferFunc func(string), extraDepth int, v ...interface{}) {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc, extraDepth)
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		addLogToBuffer(bufferFunc, extraDepth)
	} else if l.shouldLog(logLevel) {
		_ = l.inner.Log(logLevel, BuildLogEntry(v...), extraDepth)
	}
}

func logWithError(logLevel LogLevel, bufferFunc func(string), fallbackStderr bool, extraDepth int, v ...interface{}) error {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc, extraDepth)
		err := formatError(v...)
		if fallbackStderr {
			fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
		}
		return err
	}

	l.l.Lock()

	isInnerNil := l.inner == nil

	if isInnerNil {
		if !fallbackStderr {
			addLogToBuffer(bufferFunc, extraDepth)
		}
	} else if l.shouldLog(logLevel) {
		defer l.l.Unlock()
		s := BuildLogEntry(v...)
		return l.inner.Log(logLevel, s, extraDepth)
	}

	l.l.Unlock()

	err := formatError(v...)
	// Originally (PR 6436) fallbackStderr check had been added to handle a small window
	// where error messages had been lost before Logger had been initialized. Adjusting
	// just for that case because if the error log should not be logged - because it has
	// been suppressed then it should be taken into account.
	if fallbackStderr && isInnerNil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
	}
	return err
}

/*
*	logFormat functions
 */

func logFormat(logLevel LogLevel, bufferFunc func(string), extraDepth int, format string, params ...interface{}) {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc, extraDepth)
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		addLogToBuffer(bufferFunc, extraDepth)
	} else if l.shouldLog(logLevel) {
		_ = l.inner.Logf(logLevel, format, extraDepth, params...)
	}
}

func logFormatWithError(logLevel LogLevel, bufferFunc func(string), format string, fallbackStderr bool, extraDepth int, params ...interface{}) error {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc, extraDepth)
		err := formatErrorf(format, params...)
		if fallbackStderr {
			fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
		}
		return err
	}

	l.l.Lock()

	isInnerNil := l.inner == nil

	if isInnerNil {
		if !fallbackStderr {
			addLogToBuffer(bufferFunc, extraDepth)
		}
	} else if l.shouldLog(logLevel) {
		defer l.l.Unlock()
		return l.inner.Logf(logLevel, format, extraDepth, params...)
	}

	l.l.Unlock()

	err := formatErrorf(format, params...)
	// Originally (PR 6436) fallbackStderr check had been added to handle a small window
	// where error messages had been lost before Logger had been initialized. Adjusting
	// just for that case because if the error log should not be logged - because it has
	// been suppressed then it should be taken into account.
	if fallbackStderr && isInnerNil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
	}
	return err
}

/*
*	logContext functions
 */

func logContext(logLevel LogLevel, bufferFunc func(string), message string, extraDepth int, context ...interface{}) {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc, extraDepth)
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		addLogToBuffer(bufferFunc, extraDepth)
	} else if l.shouldLog(logLevel) {
		_ = l.inner.Log(logLevel, message, extraDepth, context...)
	}
}

func logContextWithError(logLevel LogLevel, bufferFunc func(string), message string, fallbackStderr bool, extraDepth int, context ...interface{}) error {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc, extraDepth)
		err := formatErrorc(message, context...)
		if fallbackStderr {
			fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
		}
		return err
	}

	l.l.Lock()

	isInnerNil := l.inner == nil

	if isInnerNil {
		if !fallbackStderr {
			addLogToBuffer(bufferFunc, extraDepth)
		}
	} else if l.shouldLog(logLevel) {
		defer l.l.Unlock()
		return l.inner.Log(logLevel, message, extraDepth, context...)
	}

	l.l.Unlock()

	err := formatErrorc(message, context...)
	// Originally (PR 6436) fallbackStderr check had been added to handle a small window
	// where error messages had been lost before Logger had been initialized. Adjusting
	// just for that case because if the error log should not be logged - because it has
	// been suppressed then it should be taken into account.
	if fallbackStderr && isInnerNil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
	}
	return err
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

func formatErrorf(format string, params ...interface{}) error {
	msg := scrubMessage(fmt.Sprintf(format, params...))
	return errors.New(msg)
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
	log(TraceLvl, func(callerPos string) { Trace(callerPos + BuildLogEntry(v...)) }, helperStackDepth, v...)
}

// Tracef logs with format at the trace level
func Tracef(format string, params ...interface{}) {
	logFormat(TraceLvl, func(callerPos string) { Trace(callerPos + fmt.Sprintf(format, params...)) }, helperStackDepth, format, params...)
}

// TracefStackDepth logs with format at the trace level and the current stack depth plus the given depth
func TracefStackDepth(depth int, format string, params ...interface{}) {
	if !ShouldLog(TraceLvl) {
		return
	}
	logFormat(TraceLvl, func(callerPos string) { Trace(callerPos + fmt.Sprintf(format, params...)) }, helperStackDepth+depth-1, format, params...)
}

// TracecStackDepth logs at the trace level with context and the current stack depth plus the additional given one
func TracecStackDepth(message string, depth int, context ...interface{}) {
	logContext(TraceLvl, func(callerPos string) { Tracec(callerPos+message, context...) }, message, helperStackDepth+depth, context...)
}

// Tracec logs at the trace level with context
func Tracec(message string, context ...interface{}) {
	TracecStackDepth(message, 1, context...)
}

// TraceFunc calls and logs the result of 'logFunc' if and only if Trace (or more verbose) logs are enabled
func TraceFunc(logFunc func() string) {
	if ShouldLog(TraceLvl) {
		TraceStackDepth(2, logFunc())
	}
}

// Debug logs at the debug level
func Debug(v ...interface{}) {
	log(DebugLvl, func(callerPos string) { Debug(callerPos + BuildLogEntry(v...)) }, helperStackDepth, v...)
}

// Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	logFormat(DebugLvl, func(callerPos string) { Debug(callerPos + fmt.Sprintf(format, params...)) }, helperStackDepth, format, params...)
}

// DebugfStackDepth logs with format at the debug level and the current stack depth plus the given depth
func DebugfStackDepth(depth int, format string, params ...interface{}) {
	if !ShouldLog(DebugLvl) {
		return
	}
	logFormat(DebugLvl, func(callerPos string) { Debug(callerPos + fmt.Sprintf(format, params...)) }, helperStackDepth+depth-1, format, params...)
}

// DebugcStackDepth logs at the debug level with context and the current stack depth plus the additional given one
func DebugcStackDepth(message string, depth int, context ...interface{}) {
	logContext(DebugLvl, func(callerPos string) { Debugc(callerPos+message, context...) }, message, helperStackDepth+depth, context...)
}

// Debugc logs at the debug level with context
func Debugc(message string, context ...interface{}) {
	DebugcStackDepth(message, 1, context...)
}

// DebugFunc calls and logs the result of 'logFunc' if and only if Debug (or more verbose) logs are enabled
func DebugFunc(logFunc func() string) {
	if ShouldLog(DebugLvl) {
		DebugStackDepth(2, logFunc())
	}
}

// Info logs at the info level
func Info(v ...interface{}) {
	log(InfoLvl, func(callerPos string) { Info(callerPos + BuildLogEntry(v...)) }, helperStackDepth, v...)
}

// Infof logs with format at the info level
func Infof(format string, params ...interface{}) {
	logFormat(InfoLvl, func(callerPos string) { Info(callerPos + fmt.Sprintf(format, params...)) }, helperStackDepth, format, params...)
}

// InfofStackDepth logs with format at the info level and the current stack depth plus the given depth
func InfofStackDepth(depth int, format string, params ...interface{}) {
	if !ShouldLog(InfoLvl) {
		return
	}
	logFormat(InfoLvl, func(callerPos string) { Info(callerPos + fmt.Sprintf(format, params...)) }, helperStackDepth+depth-1, format, params...)
}

// InfocStackDepth logs at the info level with context and the current stack depth plus the additional given one
func InfocStackDepth(message string, depth int, context ...interface{}) {
	logContext(InfoLvl, func(callerPos string) { Infoc(callerPos+message, context...) }, message, helperStackDepth+depth, context...)
}

// Infoc logs at the info level with context
func Infoc(message string, context ...interface{}) {
	InfocStackDepth(message, 1, context...)
}

// InfoFunc calls and logs the result of 'logFunc' if and only if Info (or more verbose) logs are enabled
func InfoFunc(logFunc func() string) {
	if ShouldLog(InfoLvl) {
		InfoStackDepth(2, logFunc())
	}
}

// Warn logs at the warn level and returns an error containing the formated log message
func Warn(v ...interface{}) error {
	return logWithError(WarnLvl, func(callerPos string) { _ = Warn(callerPos + BuildLogEntry(v...)) }, false, helperStackDepth, v...)
}

// Warnf logs with format at the warn level and returns an error containing the formated log message
func Warnf(format string, params ...interface{}) error {
	return logFormatWithError(WarnLvl, func(callerPos string) { _ = Warn(callerPos + fmt.Sprintf(format, params...)) }, format, false, helperStackDepth, params...)
}

// WarnfStackDepth logs with format at the warn level and the current stack depth plus the given depth
func WarnfStackDepth(depth int, format string, params ...interface{}) error {
	return logFormatWithError(WarnLvl, func(callerPos string) { _ = Warn(callerPos + fmt.Sprintf(format, params...)) }, format, false, helperStackDepth+depth-1, params...)
}

// WarncStackDepth logs at the warn level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarncStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(WarnLvl, func(callerPos string) { _ = Warnc(callerPos+message, context...) }, message, false, helperStackDepth+depth, context...)
}

// Warnc logs at the warn level with context and returns an error containing the formated log message
func Warnc(message string, context ...interface{}) error {
	return WarncStackDepth(message, 1, context...)
}

// WarnFunc calls and logs the result of 'logFunc' if and only if Warn (or more verbose) logs are enabled
func WarnFunc(logFunc func() string) {
	if ShouldLog(WarnLvl) {
		_ = WarnStackDepth(2, logFunc())
	}
}

// Error logs at the error level and returns an error containing the formated log message
func Error(v ...interface{}) error {
	return logWithError(ErrorLvl, func(callerPos string) { _ = Error(callerPos + BuildLogEntry(v...)) }, true, helperStackDepth, v...)
}

// Errorf logs with format at the error level and returns an error containing the formated log message
func Errorf(format string, params ...interface{}) error {
	return logFormatWithError(ErrorLvl, func(callerPos string) { _ = Error(callerPos + fmt.Sprintf(format, params...)) }, format, true, helperStackDepth, params...)
}

// ErrorfStackDepth logs with format at the error level and the current stack depth plus the given depth
func ErrorfStackDepth(depth int, format string, params ...interface{}) error {
	return logFormatWithError(ErrorLvl, func(callerPos string) { _ = Error(callerPos + fmt.Sprintf(format, params...)) }, format, true, helperStackDepth+depth-1, params...)
}

// ErrorcStackDepth logs at the error level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorcStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(ErrorLvl, func(callerPos string) { _ = Errorc(callerPos+message, context...) }, message, true, helperStackDepth+depth, context...)
}

// Errorc logs at the error level with context and returns an error containing the formated log message
func Errorc(message string, context ...interface{}) error {
	return ErrorcStackDepth(message, 1, context...)
}

// ErrorFunc calls and logs the result of 'logFunc' if and only if Error (or more verbose) logs are enabled
func ErrorFunc(logFunc func() string) {
	if ShouldLog(ErrorLvl) {
		_ = ErrorStackDepth(2, logFunc())
	}
}

// Critical logs at the critical level and returns an error containing the formated log message
func Critical(v ...interface{}) error {
	return logWithError(CriticalLvl, func(callerPos string) { _ = Critical(callerPos + BuildLogEntry(v...)) }, true, helperStackDepth, v...)
}

// Criticalf logs with format at the critical level and returns an error containing the formated log message
func Criticalf(format string, params ...interface{}) error {
	return logFormatWithError(CriticalLvl, func(callerPos string) { _ = Critical(callerPos + fmt.Sprintf(format, params...)) }, format, true, helperStackDepth, params...)
}

// CriticalfStackDepth logs with format at the critical level and the current stack depth plus the given depth
func CriticalfStackDepth(depth int, format string, params ...interface{}) error {
	return logFormatWithError(CriticalLvl, func(callerPos string) { _ = Critical(callerPos + fmt.Sprintf(format, params...)) }, format, false, helperStackDepth+depth-1, params...)
}

// CriticalcStackDepth logs at the critical level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalcStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(CriticalLvl, func(callerPos string) { _ = Criticalc(callerPos+message, context...) }, message, true, helperStackDepth+depth, context...)
}

// Criticalc logs at the critical level with context and returns an error containing the formated log message
func Criticalc(message string, context ...interface{}) error {
	return CriticalcStackDepth(message, 1, context...)
}

// CriticalFunc calls and logs the result of 'logFunc' if and only if Critical (or more verbose) logs are enabled
func CriticalFunc(logFunc func() string) {
	if ShouldLog(CriticalLvl) {
		_ = CriticalStackDepth(2, logFunc())
	}
}

// InfoStackDepth logs at the info level and the current stack depth plus the additional given one
func InfoStackDepth(depth int, v ...interface{}) {
	log(InfoLvl, func(callerPos string) { Info(callerPos + BuildLogEntry(v...)) }, helperStackDepth+depth-1, v...)
}

// WarnStackDepth logs at the warn level and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarnStackDepth(depth int, v ...interface{}) error {
	return logWithError(WarnLvl, func(callerPos string) { _ = Warn(callerPos + BuildLogEntry(v...)) }, false, helperStackDepth+depth-1, v...)
}

// DebugStackDepth logs at the debug level and the current stack depth plus the additional given one
func DebugStackDepth(depth int, v ...interface{}) {
	log(DebugLvl, func(callerPos string) { Debug(callerPos + BuildLogEntry(v...)) }, helperStackDepth+depth-1, v...)
}

// TraceStackDepth logs at the trace level and the current stack depth plus the additional given one
func TraceStackDepth(depth int, v ...interface{}) {
	log(TraceLvl, func(callerPos string) { Trace(callerPos + BuildLogEntry(v...)) }, helperStackDepth+depth-1, v...)
}

// ErrorStackDepth logs at the error level and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorStackDepth(depth int, v ...interface{}) error {
	return logWithError(ErrorLvl, func(callerPos string) { _ = Error(callerPos + BuildLogEntry(v...)) }, true, helperStackDepth+depth-1, v...)
}

// CriticalStackDepth logs at the critical level and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalStackDepth(depth int, v ...interface{}) error {
	return logWithError(CriticalLvl, func(callerPos string) { _ = Critical(callerPos + BuildLogEntry(v...)) }, true, helperStackDepth+depth-1, v...)
}
