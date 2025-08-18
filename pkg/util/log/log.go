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
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

type loggerPointer struct {
	atomic.Pointer[DatadogLogger]
}

var (
	// Logger is the main DatadogLogger
	logger    loggerPointer
	jmxLogger loggerPointer

	// This buffer holds log lines sent to the logger before its
	// initialization. Even if initializing the logger is one of the first
	// things the agent does, we still: load the conf, resolve secrets inside,
	// compute the final proxy settings, ...
	//
	// This buffer should be very short lived.
	logsBuffer        = []func(){}
	bufferMutex       sync.Mutex
	defaultStackDepth = 3

	// for testing purposes
	scrubBytesFunc = scrubber.ScrubBytes
)

// DatadogLogger wrapper structure for seelog
type DatadogLogger struct {
	inner LoggerInterface
	level LogLevel
	l     sync.RWMutex
}

/*
*	Setup and initialization of the logger
 */

// SetupLogger setup agent wide logger
func SetupLogger(i LoggerInterface, level string) {
	logger.Store(setupCommonLogger(i, level))

	// Flush the log entries logged before initialization now that the logger is initialized
	bufferMutex.Lock()
	defer bufferMutex.Unlock()
	for _, logLine := range logsBuffer {
		logLine()
	}
	logsBuffer = []func(){}
}

func setupCommonLogger(i LoggerInterface, level string) *DatadogLogger {
	l := &DatadogLogger{
		inner: i,
	}

	lvl, ok := LogLevelFromString(level)
	if !ok {
		lvl = InfoLvl
	}
	l.level = LogLevel(lvl)

	// We're not going to call DatadogLogger directly, but using the
	// exported functions, that will give us two frames in the stack
	// trace that should be skipped to get to the original caller.
	//
	// The fact we need a constant "additional depth" means some
	// theoretical refactor to avoid duplication in the functions
	// below cannot be performed.
	_ = l.inner.SetAdditionalStackDepth(defaultStackDepth)

	return l
}

func addLogToBuffer(logHandle func()) {
	bufferMutex.Lock()
	defer bufferMutex.Unlock()

	logsBuffer = append(logsBuffer, logHandle)
}

func scrub(s string) string {
	if scrubbed, err := scrubBytesFunc([]byte(s)); err == nil {
		return string(scrubbed)
	}
	return s
}

/*
*	Operation on the **logger level**
 */

// ChangeLogLevel changes the current log level, valid levels are trace, debug,
// info, warn, error, critical and off, it requires a new seelog logger because
// an existing one cannot be updated
func ChangeLogLevel(li LoggerInterface, level string) error {
	if err := logger.changeLogLevel(level); err != nil {
		return err
	}

	// See detailed explanation in SetupLogger(...)
	if err := li.SetAdditionalStackDepth(defaultStackDepth); err != nil {
		return err
	}

	logger.replaceInnerLogger(li)
	return nil

	// need to return something, just set to Info (expected default)
}
func (sw *loggerPointer) changeLogLevel(level string) error {
	l := sw.Load()
	if l == nil {
		return errors.New("cannot change loglevel: logger not initialized")
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		return errors.New("cannot change loglevel: logger is initialized however logger.inner is nil")
	}

	lvl, ok := LogLevelFromString(strings.ToLower(level))
	if !ok {
		return errors.New("bad log level")
	}
	l.level = LogLevel(lvl)
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

	return l.level, nil
}

// ShouldLog returns whether a given log level should be logged by the default logger
func ShouldLog(lvl LogLevel) bool {
	// The lock stay in the exported function due to the use of `shouldLog` in function that already hold the lock
	l := logger.Load()
	if l != nil {
		l.l.RLock()
		defer l.l.RUnlock()
		return l.shouldLog(lvl)
	}
	return false
}

// This function should be called with `sw.l` held
func (sw *DatadogLogger) shouldLog(level LogLevel) bool {
	return level >= sw.level
}

// ValidateLogLevel validates the given log level and returns the corresponding Seelog log level.
// If the log level is "warning", it is converted to "warn" to handle a common gotcha when used with agent5.
// If the log level is not recognized, an error is returned.
func ValidateLogLevel(logLevel string) (string, error) {
	seelogLogLevel := strings.ToLower(logLevel)
	if seelogLogLevel == "warning" { // Common gotcha when used to agent5
		seelogLogLevel = "warn"
	}

	if _, found := LogLevelFromString(seelogLogLevel); !found {
		return "", fmt.Errorf("unknown log level: %s", seelogLogLevel)
	}
	return seelogLogLevel, nil
}

/*
*	Operation on the **logger**
 */

func (sw *loggerPointer) replaceInnerLogger(li LoggerInterface) LoggerInterface {
	l := sw.Load()
	if l == nil {
		return nil // Return nil if logger is not initialized
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		return nil // Return nil if logger.inner is not initialized
	}

	old := l.inner
	l.inner = li

	return old
}

// Flush flushes the underlying inner log
func Flush() {
	logger.flush()
	jmxLogger.flush()
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

// type scrubAndLogFunc interface {
// 	ScrubAndLog(string)
// }

/*
*	log functions
 */

// log logs a message at the given level, using either bufferFunc (if logging is not yet set up) or
// scrubAndLogFunc, and treating the variadic args as the message.
func log(logLevel LogLevel, lpointer *loggerPointer, logFunc func(l LoggerInterface, s string), v ...interface{}) {
	saveForLater := func() {
		addLogToBuffer(func() {
			log(logLevel, lpointer, logFunc, v...)
		})
	}

	l := lpointer.Load()

	if l == nil {
		saveForLater()
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		saveForLater()
		return
	}

	if !l.shouldLog(logLevel) {
		return
	}

	s := BuildLogEntry(v...)

	logFunc(l.inner, s)
}

func logWithError(logLevel LogLevel, lpointer *loggerPointer, logFunc func(l LoggerInterface, s string) error, fallbackStderr bool, v ...interface{}) error {
	l := lpointer.Load()

	if l == nil {
		addLogToBuffer(func() {
			_ = logWithError(logLevel, lpointer, logFunc, fallbackStderr, v...)
		})
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
			addLogToBuffer(func() {
				_ = logWithError(logLevel, lpointer, logFunc, fallbackStderr, v...)
			})
		}
	} else if l.shouldLog(logLevel) {
		defer l.l.Unlock()
		s := BuildLogEntry(v...)
		return logFunc(l.inner, s)
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

func logFormat(logLevel LogLevel, lpointer *loggerPointer, logFunc func(l LoggerInterface, s string, params ...interface{}), format string, params ...interface{}) {
	saveForLater := func() {
		addLogToBuffer(func() {
			logFormat(logLevel, lpointer, logFunc, format, params...)
		})
	}

	l := lpointer.Load()

	if l == nil {
		saveForLater()
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		saveForLater()
		return
	}

	if !l.shouldLog(logLevel) {
		return
	}

	logFunc(l.inner, format, params...)
}
func logFormatWithError(logLevel LogLevel, lpointer *loggerPointer, logFunc func(l LoggerInterface, s string, params ...interface{}) error, format string, fallbackStderr bool, params ...interface{}) error {
	l := lpointer.Load()

	if l == nil {
		addLogToBuffer(func() {
			_ = logFormatWithError(logLevel, lpointer, logFunc, format, fallbackStderr, params...)
		})
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
			addLogToBuffer(func() {
				logFormatWithError(logLevel, lpointer, logFunc, format, fallbackStderr, params...)
			})
		}
	} else if l.shouldLog(logLevel) {
		defer l.l.Unlock()
		return logFunc(l.inner, format, params...)
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

func logContext(logLevel LogLevel, lpointer *loggerPointer, logFunc func(l LoggerInterface, s string), message string, depth int, context ...interface{}) {
	saveForLater := func() {
		addLogToBuffer(func() {
			logContext(logLevel, lpointer, logFunc, message, depth, context...)
		})
	}

	l := lpointer.Load()

	if l == nil {
		saveForLater()
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		saveForLater()
		return
	}

	if !l.shouldLog(logLevel) {
		return
	}
	l.inner.SetContext(context)
	_ = l.inner.SetAdditionalStackDepth(defaultStackDepth + depth)
	logFunc(l.inner, message)
	l.inner.SetContext(nil)
	_ = l.inner.SetAdditionalStackDepth(defaultStackDepth)
}

func logContextWithError(logLevel LogLevel, lpointer *loggerPointer, logFunc func(l LoggerInterface, s string) error, message string, fallbackStderr bool, depth int, context ...interface{}) error {
	l := lpointer.Load()
	if l == nil {
		addLogToBuffer(func() {
			logContextWithError(logLevel, lpointer, logFunc, message, fallbackStderr, depth, context...)
		})
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
			addLogToBuffer(func() {
				logContextWithError(logLevel, lpointer, logFunc, message, fallbackStderr, depth, context...)
			})
		}
	} else if l.shouldLog(logLevel) {
		l.inner.SetContext(context)
		_ = l.inner.SetAdditionalStackDepth(defaultStackDepth + depth)
		err := logFunc(l.inner, message)
		l.inner.SetContext(nil)
		_ = l.inner.SetAdditionalStackDepth(defaultStackDepth)
		defer l.l.Unlock()
		return err
	}

	l.l.Unlock()

	err := formatErrorc(message, context...)
	if fallbackStderr && isInnerNil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
	}
	return err
}

// trace logs at the trace level, called with sw.l held
func trace(l LoggerInterface, s string) {
	scrubbed := scrub(s)
	l.Trace(scrubbed)
}

// trace logs at the trace level and the current stack depth plus the
// additional given one, called with sw.l held
func traceStackDepth(l LoggerInterface, s string, depth int) {
	scrubbed := scrub(s)

	_ = l.SetAdditionalStackDepth(defaultStackDepth + depth)
	l.Trace(scrubbed)
	_ = l.SetAdditionalStackDepth(defaultStackDepth)
}

// debug logs at the debug level, called with sw.l held
func debug(l LoggerInterface, s string) {
	scrubbed := scrub(s)
	l.Debug(scrubbed)
}

// debug logs at the debug level and the current stack depth plus the additional given one, called with sw.l held
func debugStackDepth(l LoggerInterface, s string, depth int) {
	scrubbed := scrub(s)
	_ = l.SetAdditionalStackDepth(defaultStackDepth + depth)
	l.Debug(scrubbed)
	_ = l.SetAdditionalStackDepth(defaultStackDepth)
}

// info logs at the info level, called with sw.l held
func info(l LoggerInterface, s string) {
	scrubbed := scrub(s)
	l.Info(scrubbed)
}

// info logs at the info level and the current stack depth plus the additional given one, called with sw.l held
func infoStackDepth(l LoggerInterface, s string, depth int) {
	scrubbed := scrub(s)
	_ = l.SetAdditionalStackDepth(defaultStackDepth + depth)
	l.Info(scrubbed)
	_ = l.SetAdditionalStackDepth(defaultStackDepth)
}

// warn logs at the warn level, called with sw.l held
func warn(l LoggerInterface, s string) error {
	scrubbed := scrub(s)
	err := l.Warn(scrubbed)

	return err
}

// error logs at the error level and the current stack depth plus the additional given one, called with sw.l held
func warnStackDepth(l LoggerInterface, s string, depth int) error {
	scrubbed := scrub(s)
	_ = l.SetAdditionalStackDepth(defaultStackDepth + depth)
	err := l.Warn(scrubbed)
	_ = l.SetAdditionalStackDepth(defaultStackDepth)

	return err
}

// error logs at the error level, called with sw.l held
func logError(l LoggerInterface, s string) error {
	scrubbed := scrub(s)
	err := l.Error(scrubbed)

	return err
}

// error logs at the error level and the current stack depth plus the additional given one, called with sw.l held
func errorStackDepth(l LoggerInterface, s string, depth int) error {
	scrubbed := scrub(s)
	_ = l.SetAdditionalStackDepth(defaultStackDepth + depth)
	err := l.Error(scrubbed)
	_ = l.SetAdditionalStackDepth(defaultStackDepth)

	return err
}

// critical logs at the critical level, called with sw.l held
func critical(l LoggerInterface, s string) error {
	scrubbed := scrub(s)
	err := l.Critical(scrubbed)

	return err
}

// critical logs at the critical level and the current stack depth plus the additional given one, called with sw.l held
func criticalStackDepth(l LoggerInterface, s string, depth int) error {
	scrubbed := scrub(s)
	_ = l.SetAdditionalStackDepth(defaultStackDepth + depth)
	err := l.Critical(scrubbed)
	_ = l.SetAdditionalStackDepth(defaultStackDepth)

	return err
}

// tracef logs with format at the trace level, called with sw.l held
func tracef(l LoggerInterface, format string, params ...interface{}) {
	scrubbed := scrub(fmt.Sprintf(format, params...))
	l.Trace(scrubbed)
}

// debugf logs with format at the debug level, called with sw.l held
func debugf(l LoggerInterface, format string, params ...interface{}) {
	scrubbed := scrub(fmt.Sprintf(format, params...))
	l.Debug(scrubbed)
}

// infof logs with format at the info level, called with sw.l held
func infof(l LoggerInterface, format string, params ...interface{}) {
	scrubbed := scrub(fmt.Sprintf(format, params...))
	l.Info(scrubbed)
}

// warnf logs with format at the warn level, called with sw.l held
func warnf(l LoggerInterface, format string, params ...interface{}) error {
	scrubbed := scrub(fmt.Sprintf(format, params...))
	err := l.Warn(scrubbed)

	return err
}

// errorf logs with format at the error level, called with sw.l held
func errorf(l LoggerInterface, format string, params ...interface{}) error {
	scrubbed := scrub(fmt.Sprintf(format, params...))
	err := l.Error(scrubbed)

	return err
}

// criticalf logs with format at the critical level, called with sw.l held
func criticalf(l LoggerInterface, format string, params ...interface{}) error {
	scrubbed := scrub(fmt.Sprintf(format, params...))
	err := l.Critical(scrubbed)

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
	log(TraceLvl, &logger, trace, v...)
}

// Tracef logs with format at the trace level
func Tracef(format string, params ...interface{}) {
	logFormat(TraceLvl, &logger, tracef, format, params...)
}

// TracefStackDepth logs with format at the trace level and the current stack depth plus the given depth
func TracefStackDepth(depth int, format string, params ...interface{}) {
	currentLevel, _ := GetLogLevel()
	if currentLevel > TraceLvl {
		return
	}
	msg := fmt.Sprintf(format, params...)
	log(TraceLvl, &logger, func(l LoggerInterface, s string) {
		traceStackDepth(l, s, depth)
	}, msg)
}

// TracecStackDepth logs at the trace level with context and the current stack depth plus the additional given one
func TracecStackDepth(message string, depth int, context ...interface{}) {
	logContext(TraceLvl, &logger, trace, message, depth, context...)
}

// Tracec logs at the trace level with context
func Tracec(message string, context ...interface{}) {
	TracecStackDepth(message, 1, context...)
}

// TraceFunc calls and logs the result of 'logFunc' if and only if Trace (or more verbose) logs are enabled
func TraceFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= TraceLvl {
		TraceStackDepth(2, logFunc())
	}
}

// Debug logs at the debug level
func Debug(v ...interface{}) {
	log(DebugLvl, &logger, debug, v...)
}

// Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	logFormat(DebugLvl, &logger, debugf, format, params...)
}

// DebugfStackDepth logs with format at the debug level and the current stack depth plus the given depth
func DebugfStackDepth(depth int, format string, params ...interface{}) {
	currentLevel, _ := GetLogLevel()
	if currentLevel > DebugLvl {
		return
	}
	msg := fmt.Sprintf(format, params...)
	log(DebugLvl, &logger, func(l LoggerInterface, s string) {
		debugStackDepth(l, s, depth)
	}, msg)
}

// DebugcStackDepth logs at the debug level with context and the current stack depth plus the additional given one
func DebugcStackDepth(message string, depth int, context ...interface{}) {
	logContext(DebugLvl, &logger, debug, message, depth, context...)
}

// Debugc logs at the debug level with context
func Debugc(message string, context ...interface{}) {
	DebugcStackDepth(message, 1, context...)
}

// DebugFunc calls and logs the result of 'logFunc' if and only if Debug (or more verbose) logs are enabled
func DebugFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= DebugLvl {
		DebugStackDepth(2, logFunc())
	}
}

// Info logs at the info level
func Info(v ...interface{}) {
	log(InfoLvl, &logger, info, v...)
}

// Infof logs with format at the info level
func Infof(format string, params ...interface{}) {
	logFormat(InfoLvl, &logger, infof, format, params...)
}

// InfofStackDepth logs with format at the info level and the current stack depth plus the given depth
func InfofStackDepth(depth int, format string, params ...interface{}) {
	currentLevel, _ := GetLogLevel()
	if currentLevel > InfoLvl {
		return
	}
	msg := fmt.Sprintf(format, params...)
	log(InfoLvl, &logger, func(l LoggerInterface, s string) {
		infoStackDepth(l, s, depth)
	}, msg)
}

// InfocStackDepth logs at the info level with context and the current stack depth plus the additional given one
func InfocStackDepth(message string, depth int, context ...interface{}) {
	logContext(InfoLvl, &logger, info, message, depth, context...)
}

// Infoc logs at the info level with context
func Infoc(message string, context ...interface{}) {
	InfocStackDepth(message, 1, context...)
}

// InfoFunc calls and logs the result of 'logFunc' if and only if Info (or more verbose) logs are enabled
func InfoFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= InfoLvl {
		InfoStackDepth(2, logFunc())
	}
}

// Warn logs at the warn level and returns an error containing the formated log message
func Warn(v ...interface{}) error {
	return logWithError(WarnLvl, &logger, warn, false, v...)
}

// Warnf logs with format at the warn level and returns an error containing the formated log message
func Warnf(format string, params ...interface{}) error {
	return logFormatWithError(WarnLvl, &logger, warnf, format, false, params...)
}

// WarnfStackDepth logs with format at the warn level and the current stack depth plus the given depth
func WarnfStackDepth(depth int, format string, params ...interface{}) error {
	msg := fmt.Sprintf(format, params...)
	return logWithError(WarnLvl, &logger, func(l LoggerInterface, s string) error {
		return warnStackDepth(l, s, depth)
	}, false, msg)
}

// WarncStackDepth logs at the warn level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarncStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(WarnLvl, &logger, warn, message, false, depth, context...)
}

// Warnc logs at the warn level with context and returns an error containing the formated log message
func Warnc(message string, context ...interface{}) error {
	return WarncStackDepth(message, 1, context...)
}

// WarnFunc calls and logs the result of 'logFunc' if and only if Warn (or more verbose) logs are enabled
func WarnFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= WarnLvl {
		_ = WarnStackDepth(2, logFunc())
	}
}

// Error logs at the error level and returns an error containing the formated log message
func Error(v ...interface{}) error {
	return logWithError(ErrorLvl, &logger, logError, true, v...)
}

// Errorf logs with format at the error level and returns an error containing the formated log message
func Errorf(format string, params ...interface{}) error {
	return logFormatWithError(ErrorLvl, &logger, errorf, format, true, params...)
}

// ErrorfStackDepth logs with format at the error level and the current stack depth plus the given depth
func ErrorfStackDepth(depth int, format string, params ...interface{}) error {
	msg := fmt.Sprintf(format, params...)
	return logWithError(ErrorLvl, &logger, func(l LoggerInterface, s string) error {
		return errorStackDepth(l, s, depth)
	}, true, msg)
}

// ErrorcStackDepth logs at the error level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorcStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(ErrorLvl, &logger, logError, message, true, depth, context...)
}

// Errorc logs at the error level with context and returns an error containing the formated log message
func Errorc(message string, context ...interface{}) error {
	return ErrorcStackDepth(message, 1, context...)
}

// ErrorFunc calls and logs the result of 'logFunc' if and only if Error (or more verbose) logs are enabled
func ErrorFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= ErrorLvl {
		_ = ErrorStackDepth(2, logFunc())
	}
}

// Critical logs at the critical level and returns an error containing the formated log message
func Critical(v ...interface{}) error {
	return logWithError(CriticalLvl, &logger, critical, true, v...)
}

// Criticalf logs with format at the critical level and returns an error containing the formated log message
func Criticalf(format string, params ...interface{}) error {
	return logFormatWithError(CriticalLvl, &logger, criticalf, format, true, params...)
}

// CriticalfStackDepth logs with format at the critical level and the current stack depth plus the given depth
func CriticalfStackDepth(depth int, format string, params ...interface{}) error {
	msg := fmt.Sprintf(format, params...)
	return logWithError(CriticalLvl, &logger, func(l LoggerInterface, s string) error {
		return criticalStackDepth(l, s, depth)
	}, false, msg)
}

// CriticalcStackDepth logs at the critical level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalcStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(CriticalLvl, &logger, critical, message, true, depth, context...)
}

// Criticalc logs at the critical level with context and returns an error containing the formated log message
func Criticalc(message string, context ...interface{}) error {
	return CriticalcStackDepth(message, 1, context...)
}

// CriticalFunc calls and logs the result of 'logFunc' if and only if Critical (or more verbose) logs are enabled
func CriticalFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= CriticalLvl {
		_ = CriticalStackDepth(2, logFunc())
	}
}

// InfoStackDepth logs at the info level and the current stack depth plus the additional given one
func InfoStackDepth(depth int, v ...interface{}) {
	log(InfoLvl, &logger, func(l LoggerInterface, s string) {
		infoStackDepth(l, s, depth)
	}, v...)
}

// WarnStackDepth logs at the warn level and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarnStackDepth(depth int, v ...interface{}) error {
	return logWithError(WarnLvl, &logger, func(l LoggerInterface, s string) error {
		return warnStackDepth(l, s, depth)
	}, false, v...)
}

// DebugStackDepth logs at the debug level and the current stack depth plus the additional given one and returns an error containing the formated log message
func DebugStackDepth(depth int, v ...interface{}) {
	log(DebugLvl, &logger, func(l LoggerInterface, s string) {
		debugStackDepth(l, s, depth)
	}, v...)
}

// TraceStackDepth logs at the trace level and the current stack depth plus the additional given one and returns an error containing the formated log message
func TraceStackDepth(depth int, v ...interface{}) {
	log(TraceLvl, &logger, func(l LoggerInterface, s string) {
		traceStackDepth(l, s, depth)
	}, v...)
}

// ErrorStackDepth logs at the error level and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorStackDepth(depth int, v ...interface{}) error {
	return logWithError(ErrorLvl, &logger, func(l LoggerInterface, s string) error {
		return errorStackDepth(l, s, depth)
	}, true, v...)
}

// CriticalStackDepth logs at the critical level and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalStackDepth(depth int, v ...interface{}) error {
	return logWithError(CriticalLvl, &logger, func(l LoggerInterface, s string) error {
		return criticalStackDepth(l, s, depth)
	}, true, v...)
}

/*
*	JMX Logger Section
 */

// JMXError Logs for JMX check
func JMXError(v ...interface{}) error {
	return logWithError(ErrorLvl, &jmxLogger, logError, true, v...)
}

// JMXInfo Logs
func JMXInfo(v ...interface{}) {
	log(InfoLvl, &jmxLogger, info, v...)
}

// SetupJMXLogger setup JMXfetch specific logger
func SetupJMXLogger(i LoggerInterface, level string) {
	jmxLogger.Store(setupCommonLogger(i, level))
}
