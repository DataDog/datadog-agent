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

	"github.com/cihub/seelog"
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
	inner seelog.LoggerInterface
	level seelog.LogLevel
	extra map[string]seelog.LoggerInterface
	l     sync.RWMutex
}

/*
*	Setup and initialization of the logger
 */

// SetupLogger setup agent wide logger
func SetupLogger(i seelog.LoggerInterface, level string) {
	logger.Store(setupCommonLogger(i, level))

	// Flush the log entries logged before initialization now that the logger is initialized
	bufferMutex.Lock()
	defer bufferMutex.Unlock()
	for _, logLine := range logsBuffer {
		logLine()
	}
	logsBuffer = []func(){}
}

func setupCommonLogger(i seelog.LoggerInterface, level string) *DatadogLogger {
	l := &DatadogLogger{
		inner: i,
		extra: make(map[string]seelog.LoggerInterface),
	}

	lvl, ok := seelog.LogLevelFromString(level)
	if !ok {
		lvl = seelog.InfoLvl
	}
	l.level = lvl

	// We're not going to call DatadogLogger directly, but using the
	// exported functions, that will give us two frames in the stack
	// trace that should be skipped to get to the original caller.
	//
	// The fact we need a constant "additional depth" means some
	// theoretical refactor to avoid duplication in the functions
	// below cannot be performed.
	l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	return l
}

func addLogToBuffer(logHandle func()) {
	bufferMutex.Lock()
	defer bufferMutex.Unlock()

	logsBuffer = append(logsBuffer, logHandle)
}

func (sw *DatadogLogger) scrub(s string) string {
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
func ChangeLogLevel(li seelog.LoggerInterface, level string) error {
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

	lvl, ok := seelog.LogLevelFromString(strings.ToLower(level))
	if !ok {
		return errors.New("bad log level")
	}
	l.level = lvl
	return nil
}

// GetLogLevel returns a seelog native representation of the current log level
func GetLogLevel() (seelog.LogLevel, error) {
	return logger.getLogLevel()
}
func (sw *loggerPointer) getLogLevel() (seelog.LogLevel, error) {
	l := sw.Load()
	if l == nil {
		return seelog.InfoLvl, errors.New("cannot get loglevel: logger not initialized")
	}

	l.l.RLock()
	defer l.l.RUnlock()

	if l.inner == nil {
		return seelog.InfoLvl, errors.New("cannot get loglevel: logger not initialized")
	}

	return l.level, nil
}

// ShouldLog returns whether a given log level should be logged by the default logger
func ShouldLog(lvl seelog.LogLevel) bool {
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
func (sw *DatadogLogger) shouldLog(level seelog.LogLevel) bool {
	return level >= sw.level
}

/*
*	Operation on the **logger**
 */

// RegisterAdditionalLogger registers an additional logger for logging
func RegisterAdditionalLogger(n string, li seelog.LoggerInterface) error {
	return logger.registerAdditionalLogger(n, li)
}
func (sw *loggerPointer) registerAdditionalLogger(n string, li seelog.LoggerInterface) error {
	l := sw.Load()
	if l == nil {
		return errors.New("cannot register: logger not initialized")
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		return errors.New("cannot register: logger not initialized")
	}

	if l.extra == nil {

		return errors.New("logger not fully initialized, additional logging unavailable")
	}

	if _, ok := l.extra[n]; ok {
		return errors.New("logger already registered with that name")
	}
	l.extra[n] = li

	return nil
}

// ReplaceLogger allows replacing the internal logger, returns old logger
func ReplaceLogger(li seelog.LoggerInterface) seelog.LoggerInterface {
	return logger.replaceInnerLogger(li)
}
func (sw *loggerPointer) replaceInnerLogger(li seelog.LoggerInterface) seelog.LoggerInterface {
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

/*
*	log functions
 */

// log logs a message at the given level, using either bufferFunc (if logging is not yet set up) or
// scrubAndLogFunc, and treating the variadic args as the message.
func log(logLevel seelog.LogLevel, bufferFunc func(), scrubAndLogFunc func(string), v ...interface{}) {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc)
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		addLogToBuffer(bufferFunc)
	} else if l.shouldLog(logLevel) {
		s := BuildLogEntry(v...)
		scrubAndLogFunc(s)
	}

}
func logWithError(logLevel seelog.LogLevel, bufferFunc func(), scrubAndLogFunc func(string) error, fallbackStderr bool, v ...interface{}) error {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc)
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
			addLogToBuffer(bufferFunc)
		}
	} else if l.shouldLog(logLevel) {
		defer l.l.Unlock()
		s := BuildLogEntry(v...)
		return scrubAndLogFunc(s)
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

func logFormat(logLevel seelog.LogLevel, bufferFunc func(), scrubAndLogFunc func(string, ...interface{}), format string, params ...interface{}) {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc)
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		addLogToBuffer(bufferFunc)
	} else if l.shouldLog(logLevel) {
		scrubAndLogFunc(format, params...)
	}
}
func logFormatWithError(logLevel seelog.LogLevel, bufferFunc func(), scrubAndLogFunc func(string, ...interface{}) error, format string, fallbackStderr bool, params ...interface{}) error {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc)
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
			addLogToBuffer(bufferFunc)
		}
	} else if l.shouldLog(logLevel) {
		defer l.l.Unlock()
		return scrubAndLogFunc(format, params...)
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

func logContext(logLevel seelog.LogLevel, bufferFunc func(), scrubAndLogFunc func(string), message string, depth int, context ...interface{}) {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc)
		return
	}

	l.l.Lock()
	defer l.l.Unlock()

	if l.inner == nil {
		addLogToBuffer(bufferFunc)
	} else if l.shouldLog(logLevel) {
		l.inner.SetContext(context)
		l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
		scrubAndLogFunc(message)
		l.inner.SetContext(nil)
		l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck
	}
}
func logContextWithError(logLevel seelog.LogLevel, bufferFunc func(), scrubAndLogFunc func(string) error, message string, fallbackStderr bool, depth int, context ...interface{}) error {
	l := logger.Load()

	if l == nil {
		addLogToBuffer(bufferFunc)
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
			addLogToBuffer(bufferFunc)
		}
	} else if l.shouldLog(logLevel) {
		l.inner.SetContext(context)
		l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
		err := scrubAndLogFunc(message)
		l.inner.SetContext(nil)
		l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck
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
func (sw *loggerPointer) trace(s string) {
	l := sw.Load()

	if l == nil {
		return
	}

	scrubbed := l.scrub(s)
	l.inner.Trace(scrubbed)

	for _, l := range l.extra {
		l.Trace(scrubbed)
	}
}

// trace logs at the trace level and the current stack depth plus the
// additional given one, called with sw.l held
func (sw *loggerPointer) traceStackDepth(s string, depth int) {
	l := sw.Load()
	scrubbed := l.scrub(s)

	l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	l.inner.Trace(scrubbed)
	l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range l.extra {
		l.Trace(scrubbed)
	}
}

// debug logs at the debug level, called with sw.l held
func (sw *loggerPointer) debug(s string) {
	l := sw.Load()
	scrubbed := l.scrub(s)
	l.inner.Debug(scrubbed)

	for _, l := range l.extra {
		l.Debug(scrubbed)
	}
}

// debug logs at the debug level and the current stack depth plus the additional given one, called with sw.l held
func (sw *loggerPointer) debugStackDepth(s string, depth int) {
	l := sw.Load()
	scrubbed := l.scrub(s)
	l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	l.inner.Debug(scrubbed)
	l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range l.extra {
		l.Debug(scrubbed) //nolint:errcheck
	}
}

// info logs at the info level, called with sw.l held
func (sw *loggerPointer) info(s string) {
	l := sw.Load()
	scrubbed := l.scrub(s)
	l.inner.Info(scrubbed)
	for _, l := range l.extra {
		l.Info(scrubbed)
	}
}

// info logs at the info level and the current stack depth plus the additional given one, called with sw.l held
func (sw *loggerPointer) infoStackDepth(s string, depth int) {
	l := sw.Load()
	scrubbed := l.scrub(s)
	l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	l.inner.Info(scrubbed)
	l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range l.extra {
		l.Info(scrubbed) //nolint:errcheck
	}
}

// warn logs at the warn level, called with sw.l held
func (sw *loggerPointer) warn(s string) error {
	l := sw.Load()
	scrubbed := l.scrub(s)
	err := l.inner.Warn(scrubbed)

	for _, l := range l.extra {
		l.Warn(scrubbed) //nolint:errcheck
	}

	return err
}

// error logs at the error level and the current stack depth plus the additional given one, called with sw.l held
func (sw *loggerPointer) warnStackDepth(s string, depth int) error {
	l := sw.Load()
	scrubbed := l.scrub(s)
	l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	err := l.inner.Warn(scrubbed)
	l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range l.extra {
		l.Warn(scrubbed) //nolint:errcheck
	}

	return err
}

// error logs at the error level, called with sw.l held
func (sw *loggerPointer) error(s string) error {
	l := sw.Load()
	scrubbed := l.scrub(s)
	err := l.inner.Error(scrubbed)

	for _, l := range l.extra {
		l.Error(scrubbed) //nolint:errcheck
	}

	return err
}

// error logs at the error level and the current stack depth plus the additional given one, called with sw.l held
func (sw *loggerPointer) errorStackDepth(s string, depth int) error {
	l := sw.Load()
	scrubbed := l.scrub(s)
	l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	err := l.inner.Error(scrubbed)
	l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range l.extra {
		l.Error(scrubbed) //nolint:errcheck
	}

	return err
}

// critical logs at the critical level, called with sw.l held
func (sw *loggerPointer) critical(s string) error {
	l := sw.Load()
	scrubbed := l.scrub(s)
	err := l.inner.Critical(scrubbed)

	for _, l := range l.extra {
		l.Critical(scrubbed) //nolint:errcheck
	}

	return err
}

// critical logs at the critical level and the current stack depth plus the additional given one, called with sw.l held
func (sw *loggerPointer) criticalStackDepth(s string, depth int) error {
	l := sw.Load()
	scrubbed := l.scrub(s)
	l.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	err := l.inner.Critical(scrubbed)
	l.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range l.extra {
		l.Critical(scrubbed) //nolint:errcheck
	}

	return err
}

// tracef logs with format at the trace level, called with sw.l held
func (sw *loggerPointer) tracef(format string, params ...interface{}) {
	l := sw.Load()
	scrubbed := l.scrub(fmt.Sprintf(format, params...))
	l.inner.Trace(scrubbed)

	for _, l := range l.extra {
		l.Trace(scrubbed)
	}
}

// debugf logs with format at the debug level, called with sw.l held
func (sw *loggerPointer) debugf(format string, params ...interface{}) {
	l := sw.Load()
	scrubbed := l.scrub(fmt.Sprintf(format, params...))
	l.inner.Debug(scrubbed)

	for _, l := range l.extra {
		l.Debug(scrubbed)
	}
}

// infof logs with format at the info level, called with sw.l held
func (sw *loggerPointer) infof(format string, params ...interface{}) {
	l := sw.Load()
	scrubbed := l.scrub(fmt.Sprintf(format, params...))
	l.inner.Info(scrubbed)

	for _, l := range l.extra {
		l.Info(scrubbed)
	}
}

// warnf logs with format at the warn level, called with sw.l held
func (sw *loggerPointer) warnf(format string, params ...interface{}) error {
	l := sw.Load()
	scrubbed := l.scrub(fmt.Sprintf(format, params...))
	err := l.inner.Warn(scrubbed)

	for _, l := range l.extra {
		l.Warn(scrubbed) //nolint:errcheck
	}

	return err
}

// errorf logs with format at the error level, called with sw.l held
func (sw *loggerPointer) errorf(format string, params ...interface{}) error {
	l := sw.Load()
	scrubbed := l.scrub(fmt.Sprintf(format, params...))
	err := l.inner.Error(scrubbed)

	for _, l := range l.extra {
		l.Error(scrubbed) //nolint:errcheck
	}

	return err
}

// criticalf logs with format at the critical level, called with sw.l held
func (sw *loggerPointer) criticalf(format string, params ...interface{}) error {
	l := sw.Load()
	scrubbed := l.scrub(fmt.Sprintf(format, params...))
	err := l.inner.Critical(scrubbed)

	for _, l := range l.extra {
		l.Critical(scrubbed) //nolint:errcheck
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
	log(seelog.TraceLvl, func() { Trace(v...) }, logger.trace, v...)
}

// Tracef logs with format at the trace level
func Tracef(format string, params ...interface{}) {
	logFormat(seelog.TraceLvl, func() { Tracef(format, params...) }, logger.tracef, format, params...)
}

// TracefStackDepth logs with format at the trace level and the current stack depth plus the given depth
func TracefStackDepth(depth int, format string, params ...interface{}) {
	currentLevel, _ := GetLogLevel()
	if currentLevel > seelog.TraceLvl {
		return
	}
	msg := fmt.Sprintf(format, params...)
	log(seelog.TraceLvl, func() { TraceStackDepth(depth, msg) }, func(s string) {
		logger.traceStackDepth(s, depth)
	}, msg)
}

// TracecStackDepth logs at the trace level with context and the current stack depth plus the additional given one
func TracecStackDepth(message string, depth int, context ...interface{}) {
	logContext(seelog.TraceLvl, func() { Tracec(message, context...) }, logger.trace, message, depth, context...)
}

// Tracec logs at the trace level with context
func Tracec(message string, context ...interface{}) {
	TracecStackDepth(message, 1, context...)
}

// TraceFunc calls and logs the result of 'logFunc' if and only if Trace (or more verbose) logs are enabled
func TraceFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= seelog.TraceLvl {
		TraceStackDepth(2, logFunc())
	}
}

// Debug logs at the debug level
func Debug(v ...interface{}) {
	log(seelog.DebugLvl, func() { Debug(v...) }, logger.debug, v...)
}

// Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	logFormat(seelog.DebugLvl, func() { Debugf(format, params...) }, logger.debugf, format, params...)
}

// DebugfStackDepth logs with format at the debug level and the current stack depth plus the given depth
func DebugfStackDepth(depth int, format string, params ...interface{}) {
	currentLevel, _ := GetLogLevel()
	if currentLevel > seelog.DebugLvl {
		return
	}
	msg := fmt.Sprintf(format, params...)
	log(seelog.DebugLvl, func() { DebugStackDepth(depth, msg) }, func(s string) {
		logger.debugStackDepth(s, depth)
	}, msg)
}

// DebugcStackDepth logs at the debug level with context and the current stack depth plus the additional given one
func DebugcStackDepth(message string, depth int, context ...interface{}) {
	logContext(seelog.DebugLvl, func() { Debugc(message, context...) }, logger.debug, message, depth, context...)
}

// Debugc logs at the debug level with context
func Debugc(message string, context ...interface{}) {
	DebugcStackDepth(message, 1, context...)
}

// DebugFunc calls and logs the result of 'logFunc' if and only if Debug (or more verbose) logs are enabled
func DebugFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= seelog.DebugLvl {
		DebugStackDepth(2, logFunc())
	}
}

// Info logs at the info level
func Info(v ...interface{}) {
	log(seelog.InfoLvl, func() { Info(v...) }, logger.info, v...)
}

// Infof logs with format at the info level
func Infof(format string, params ...interface{}) {
	logFormat(seelog.InfoLvl, func() { Infof(format, params...) }, logger.infof, format, params...)
}

// InfofStackDepth logs with format at the info level and the current stack depth plus the given depth
func InfofStackDepth(depth int, format string, params ...interface{}) {
	currentLevel, _ := GetLogLevel()
	if currentLevel > seelog.InfoLvl {
		return
	}
	msg := fmt.Sprintf(format, params...)
	log(seelog.InfoLvl, func() { InfoStackDepth(depth, msg) }, func(s string) {
		logger.infoStackDepth(s, depth)
	}, msg)
}

// InfocStackDepth logs at the info level with context and the current stack depth plus the additional given one
func InfocStackDepth(message string, depth int, context ...interface{}) {
	logContext(seelog.InfoLvl, func() { Infoc(message, context...) }, logger.info, message, depth, context...)
}

// Infoc logs at the info level with context
func Infoc(message string, context ...interface{}) {
	InfocStackDepth(message, 1, context...)
}

// InfoFunc calls and logs the result of 'logFunc' if and only if Info (or more verbose) logs are enabled
func InfoFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= seelog.InfoLvl {
		InfoStackDepth(2, logFunc())
	}
}

// Warn logs at the warn level and returns an error containing the formated log message
func Warn(v ...interface{}) error {
	return logWithError(seelog.WarnLvl, func() { Warn(v...) }, logger.warn, false, v...)
}

// Warnf logs with format at the warn level and returns an error containing the formated log message
func Warnf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.WarnLvl, func() { Warnf(format, params...) }, logger.warnf, format, false, params...)
}

// WarnfStackDepth logs with format at the warn level and the current stack depth plus the given depth
func WarnfStackDepth(depth int, format string, params ...interface{}) error {
	msg := fmt.Sprintf(format, params...)
	return logWithError(seelog.WarnLvl, func() { WarnStackDepth(depth, msg) }, func(s string) error {
		return logger.warnStackDepth(s, depth)
	}, false, msg)
}

// WarncStackDepth logs at the warn level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarncStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(seelog.WarnLvl, func() { Warnc(message, context...) }, logger.warn, message, false, depth, context...)
}

// Warnc logs at the warn level with context and returns an error containing the formated log message
func Warnc(message string, context ...interface{}) error {
	return WarncStackDepth(message, 1, context...)
}

// WarnFunc calls and logs the result of 'logFunc' if and only if Warn (or more verbose) logs are enabled
func WarnFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= seelog.WarnLvl {
		WarnStackDepth(2, logFunc())
	}
}

// Error logs at the error level and returns an error containing the formated log message
func Error(v ...interface{}) error {
	return logWithError(seelog.ErrorLvl, func() { Error(v...) }, logger.error, true, v...)
}

// Errorf logs with format at the error level and returns an error containing the formated log message
func Errorf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.ErrorLvl, func() { Errorf(format, params...) }, logger.errorf, format, true, params...)
}

// ErrorfStackDepth logs with format at the error level and the current stack depth plus the given depth
func ErrorfStackDepth(depth int, format string, params ...interface{}) error {
	msg := fmt.Sprintf(format, params...)
	return logWithError(seelog.ErrorLvl, func() { ErrorStackDepth(depth, msg) }, func(s string) error {
		return logger.errorStackDepth(s, depth)
	}, true, msg)
}

// ErrorcStackDepth logs at the error level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorcStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(seelog.ErrorLvl, func() { Errorc(message, context...) }, logger.error, message, true, depth, context...)
}

// Errorc logs at the error level with context and returns an error containing the formated log message
func Errorc(message string, context ...interface{}) error {
	return ErrorcStackDepth(message, 1, context...)
}

// ErrorFunc calls and logs the result of 'logFunc' if and only if Error (or more verbose) logs are enabled
func ErrorFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= seelog.ErrorLvl {
		ErrorStackDepth(2, logFunc())
	}
}

// Critical logs at the critical level and returns an error containing the formated log message
func Critical(v ...interface{}) error {
	return logWithError(seelog.CriticalLvl, func() { Critical(v...) }, logger.critical, true, v...)
}

// Criticalf logs with format at the critical level and returns an error containing the formated log message
func Criticalf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.CriticalLvl, func() { Criticalf(format, params...) }, logger.criticalf, format, true, params...)
}

// CriticalfStackDepth logs with format at the critical level and the current stack depth plus the given depth
func CriticalfStackDepth(depth int, format string, params ...interface{}) error {
	msg := fmt.Sprintf(format, params...)
	return logWithError(seelog.CriticalLvl, func() { CriticalStackDepth(depth, msg) }, func(s string) error {
		return logger.criticalStackDepth(s, depth)
	}, false, msg)
}

// CriticalcStackDepth logs at the critical level with context and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalcStackDepth(message string, depth int, context ...interface{}) error {
	return logContextWithError(seelog.CriticalLvl, func() { Criticalc(message, context...) }, logger.critical, message, true, depth, context...)
}

// Criticalc logs at the critical level with context and returns an error containing the formated log message
func Criticalc(message string, context ...interface{}) error {
	return CriticalcStackDepth(message, 1, context...)
}

// CriticalFunc calls and logs the result of 'logFunc' if and only if Critical (or more verbose) logs are enabled
func CriticalFunc(logFunc func() string) {
	currentLevel, _ := GetLogLevel()
	if currentLevel <= seelog.CriticalLvl {
		CriticalStackDepth(2, logFunc())
	}
}

// InfoStackDepth logs at the info level and the current stack depth plus the additional given one
func InfoStackDepth(depth int, v ...interface{}) {
	log(seelog.InfoLvl, func() { InfoStackDepth(depth, v...) }, func(s string) {
		logger.infoStackDepth(s, depth)
	}, v...)
}

// WarnStackDepth logs at the warn level and the current stack depth plus the additional given one and returns an error containing the formated log message
func WarnStackDepth(depth int, v ...interface{}) error {
	return logWithError(seelog.WarnLvl, func() { WarnStackDepth(depth, v...) }, func(s string) error {
		return logger.warnStackDepth(s, depth)
	}, false, v...)
}

// DebugStackDepth logs at the debug level and the current stack depth plus the additional given one and returns an error containing the formated log message
func DebugStackDepth(depth int, v ...interface{}) {
	log(seelog.DebugLvl, func() { DebugStackDepth(depth, v...) }, func(s string) {
		logger.debugStackDepth(s, depth)
	}, v...)
}

// TraceStackDepth logs at the trace level and the current stack depth plus the additional given one and returns an error containing the formated log message
func TraceStackDepth(depth int, v ...interface{}) {
	log(seelog.TraceLvl, func() { TraceStackDepth(depth, v...) }, func(s string) {
		logger.traceStackDepth(s, depth)
	}, v...)
}

// ErrorStackDepth logs at the error level and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorStackDepth(depth int, v ...interface{}) error {
	return logWithError(seelog.ErrorLvl, func() { ErrorStackDepth(depth, v...) }, func(s string) error {
		return logger.errorStackDepth(s, depth)
	}, true, v...)
}

// CriticalStackDepth logs at the critical level and the current stack depth plus the additional given one and returns an error containing the formated log message
func CriticalStackDepth(depth int, v ...interface{}) error {
	return logWithError(seelog.CriticalLvl, func() { CriticalStackDepth(depth, v...) }, func(s string) error {
		return logger.criticalStackDepth(s, depth)
	}, true, v...)
}

/*
*	JMX Logger Section
 */

// JMXError Logs for JMX check
func JMXError(v ...interface{}) error {
	return logWithError(seelog.ErrorLvl, func() { JMXError(v...) }, jmxLogger.error, true, v...)
}

// JMXInfo Logs
func JMXInfo(v ...interface{}) {
	log(seelog.InfoLvl, func() { JMXInfo(v...) }, jmxLogger.info, v...)
}

// SetupJMXLogger setup JMXfetch specific logger
func SetupJMXLogger(i seelog.LoggerInterface, level string) {
	jmxLogger.Store(setupCommonLogger(i, level))
}
