// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package log

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/cihub/seelog"
)

var (
	logger *DatadogLogger

	// This buffer holds log lines sent to the logger before its
	// initialization. Even if initializing the logger is one of the first
	// things the agent does, we still: load the conf, resolve secrets inside,
	// compute the final proxy settings, ...
	//
	// This buffer should be very short lived.
	logsBuffer           = []func(){}
	bufferLogsBeforeInit = true
	bufferMutex          sync.Mutex
	defaultStackDepth    = 3
)

// DatadogLogger wrapper structure for seelog
type DatadogLogger struct {
	inner       seelog.LoggerInterface
	level       seelog.LogLevel
	extra       map[string]seelog.LoggerInterface
	l           sync.RWMutex
	contextLock sync.Mutex
}

// SetupDatadogLogger configure logger singleton with seelog interface
func SetupDatadogLogger(l seelog.LoggerInterface, level string) {
	logger = &DatadogLogger{
		inner: l,
		extra: make(map[string]seelog.LoggerInterface),
	}

	lvl, ok := seelog.LogLevelFromString(level)
	if !ok {
		lvl = seelog.InfoLvl
	}
	logger.level = lvl

	// We're not going to call DatadogLogger directly, but using the
	// exported functions, that will give us two frames in the stack
	// trace that should be skipped to get to the original caller.
	//
	// The fact we need a constant "additional depth" means some
	// theoretical refactor to avoid duplication in the functions
	// below cannot be performed.
	logger.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	// Flushing logs since the logger is now initialized
	bufferMutex.Lock()
	bufferLogsBeforeInit = false
	defer bufferMutex.Unlock()
	for _, logLine := range logsBuffer {
		logLine()
	}
	logsBuffer = []func(){}
}

func addLogToBuffer(logHandle func()) {
	bufferMutex.Lock()
	defer bufferMutex.Unlock()

	logsBuffer = append(logsBuffer, logHandle)
}

func (sw *DatadogLogger) replaceInnerLogger(l seelog.LoggerInterface) seelog.LoggerInterface {
	sw.l.Lock()
	defer sw.l.Unlock()

	old := sw.inner
	sw.inner = l

	return old
}

func (sw *DatadogLogger) changeLogLevel(level string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	lvl, ok := seelog.LogLevelFromString(strings.ToLower(level))
	if !ok {
		return errors.New("bad log level")
	}
	logger.level = lvl
	return nil
}

func (sw *DatadogLogger) shouldLog(level seelog.LogLevel) bool {
	sw.l.RLock()
	shouldLog := level >= sw.level
	sw.l.RUnlock()

	return shouldLog
}

func (sw *DatadogLogger) registerAdditionalLogger(n string, l seelog.LoggerInterface) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	if sw.extra == nil {
		return errors.New("logger not fully initialized, additional logging unavailable")
	}

	if _, ok := sw.extra[n]; ok {
		return errors.New("logger already registered with that name")
	}
	sw.extra[n] = l

	return nil
}

func (sw *DatadogLogger) unregisterAdditionalLogger(n string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	if sw.extra == nil {
		return errors.New("logger not fully initialized, additional logging unavailable")
	}

	delete(sw.extra, n)
	return nil
}

func (sw *DatadogLogger) scrub(s string) string {
	if scrubbed, err := CredentialsCleanerBytes([]byte(s)); err == nil {
		return string(scrubbed)
	}

	return s
}

// trace logs at the trace level
func (sw *DatadogLogger) trace(s string) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.Trace(scrubbed)

	for _, l := range sw.extra {
		l.Trace(scrubbed)
	}
}

// debug logs at the debug level
func (sw *DatadogLogger) debug(s string) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.Debug(scrubbed)

	for _, l := range sw.extra {
		l.Debug(scrubbed)
	}
}

// info logs at the info level
func (sw *DatadogLogger) info(s string) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.Info(scrubbed)

	for _, l := range sw.extra {
		l.Info(scrubbed)
	}
}

// warn logs at the warn level
func (sw *DatadogLogger) warn(s string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	err := sw.inner.Warn(scrubbed)

	for _, l := range sw.extra {
		l.Warn(scrubbed) //nolint:errcheck
	}

	return err
}

// error logs at the error level
func (sw *DatadogLogger) error(s string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	err := sw.inner.Error(scrubbed)

	for _, l := range sw.extra {
		l.Error(scrubbed) //nolint:errcheck
	}

	return err
}

// error logs at the error level and the current stack depth plus the additional given one
func (sw *DatadogLogger) errorStackDepth(s string, depth int) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	err := sw.inner.Error(scrubbed)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range sw.extra {
		l.Error(scrubbed) //nolint:errcheck
	}

	return err
}

// critical logs at the critical level
func (sw *DatadogLogger) critical(s string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	err := sw.inner.Critical(scrubbed)

	for _, l := range sw.extra {
		l.Critical(scrubbed) //nolint:errcheck
	}

	return err
}

// tracef logs with format at the trace level
func (sw *DatadogLogger) tracef(format string, params ...interface{}) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	sw.inner.Trace(scrubbed)

	for _, l := range sw.extra {
		l.Trace(scrubbed)
	}
}

// debugf logs with format at the debug level
func (sw *DatadogLogger) debugf(format string, params ...interface{}) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	sw.inner.Debug(scrubbed)

	for _, l := range sw.extra {
		l.Debug(scrubbed)
	}
}

// infof logs with format at the info level
func (sw *DatadogLogger) infof(format string, params ...interface{}) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	sw.inner.Info(scrubbed)

	for _, l := range sw.extra {
		l.Info(scrubbed)
	}
}

// warnf logs with format at the warn level
func (sw *DatadogLogger) warnf(format string, params ...interface{}) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	err := sw.inner.Warn(scrubbed)

	for _, l := range sw.extra {
		l.Warn(scrubbed) //nolint:errcheck
	}

	return err
}

// errorf logs with format at the error level
func (sw *DatadogLogger) errorf(format string, params ...interface{}) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	err := sw.inner.Error(scrubbed)

	for _, l := range sw.extra {
		l.Error(scrubbed) //nolint:errcheck
	}

	return err
}

// criticalf logs with format at the critical level
func (sw *DatadogLogger) criticalf(format string, params ...interface{}) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	err := sw.inner.Critical(scrubbed)

	for _, l := range sw.extra {
		l.Critical(scrubbed) //nolint:errcheck
	}

	return err
}

// getLogLevel returns the current log level
func (sw *DatadogLogger) getLogLevel() seelog.LogLevel {
	sw.l.RLock()
	defer sw.l.RUnlock()

	return sw.level
}

func buildLogEntry(v ...interface{}) string {
	var fmtBuffer bytes.Buffer

	for i := 0; i < len(v)-1; i++ {
		fmtBuffer.WriteString("%v ")
	}
	fmtBuffer.WriteString("%v")

	return fmt.Sprintf(fmtBuffer.String(), v...)
}

func scrubMessage(message string) string {
	msgScrubbed, err := CredentialsCleanerBytes([]byte(message))
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

func log(logLevel seelog.LogLevel, bufferFunc func(), logFunc func(string), v ...interface{}) {
	if logger != nil && logger.inner != nil && logger.shouldLog(logLevel) {
		s := buildLogEntry(v...)
		logFunc(logger.scrub(s))
	} else if bufferLogsBeforeInit && (logger == nil || logger.inner == nil) {
		addLogToBuffer(bufferFunc)
	}
}

func logWithError(logLevel seelog.LogLevel, bufferFunc func(), logFunc func(string) error, fallbackStderr bool, v ...interface{}) error {
	if logger != nil && logger.inner != nil && logger.shouldLog(logLevel) {
		s := buildLogEntry(v...)
		return logFunc(logger.scrub(s))
	} else if bufferLogsBeforeInit && (logger == nil || logger.inner == nil) {
		addLogToBuffer(bufferFunc)
	}
	err := formatError(v...)
	if fallbackStderr {
		fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
	}
	return err
}

func logFormat(logLevel seelog.LogLevel, bufferFunc func(), logFunc func(string, ...interface{}), format string, params ...interface{}) {
	if logger != nil && logger.inner != nil && logger.shouldLog(logLevel) {
		logFunc(format, params...)
	} else if bufferLogsBeforeInit && (logger == nil || logger.inner == nil) {
		addLogToBuffer(bufferFunc)
	}
}

func logFormatWithError(logLevel seelog.LogLevel, bufferFunc func(), logFunc func(string, ...interface{}) error, format string, fallbackStderr bool, params ...interface{}) error {
	if logger != nil && logger.inner != nil && logger.shouldLog(logLevel) {
		return logFunc(format, params...)
	} else if bufferLogsBeforeInit && (logger == nil || logger.inner == nil) {
		addLogToBuffer(bufferFunc)
	}
	err := formatErrorf(format, params...)
	if fallbackStderr {
		fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
	}
	return err
}

func logContext(logLevel seelog.LogLevel, bufferFunc func(), logFunc func(string), message string, context ...interface{}) {
	if logger != nil && logger.inner != nil && logger.shouldLog(logLevel) {
		msg := logger.scrub(message)
		logger.contextLock.Lock()
		logger.inner.SetContext(context)
		logFunc(msg)
		logger.inner.SetContext(nil)
		// Not using defer to make sure we release lock as fast as possible
		logger.contextLock.Unlock()
	} else if bufferLogsBeforeInit && (logger == nil || logger.inner == nil) {
		addLogToBuffer(bufferFunc)
	}
}

// Trace logs at the trace level
func Trace(v ...interface{}) {
	log(seelog.TraceLvl, func() { Trace(v...) }, logger.trace, v...)
}

// Tracef logs with format at the trace level
func Tracef(format string, params ...interface{}) {
	logFormat(seelog.TraceLvl, func() { Tracef(format, params...) }, logger.tracef, format, params...)
}

// Tracec logs at the trace level with context
func Tracec(message string, context ...interface{}) {
	logContext(seelog.TraceLvl, func() { Tracec(message, context...) }, logger.trace, message, context...)
}

// Debug logs at the debug level
func Debug(v ...interface{}) {
	log(seelog.DebugLvl, func() { Debug(v...) }, logger.debug, v...)
}

// Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	logFormat(seelog.DebugLvl, func() { Debugf(format, params...) }, logger.debugf, format, params...)
}

// Debugc logs at the debug level with context
func Debugc(message string, context ...interface{}) {
	logContext(seelog.DebugLvl, func() { Debugc(message, context...) }, logger.debug, message, context...)
}

// Info logs at the info level
func Info(v ...interface{}) {
	log(seelog.InfoLvl, func() { Info(v...) }, logger.info, v...)
}

// Infof logs with format at the info level
func Infof(format string, params ...interface{}) {
	logFormat(seelog.InfoLvl, func() { Infof(format, params...) }, logger.infof, format, params...)
}

// Infoc logs at the info level with context
func Infoc(message string, context ...interface{}) {
	logContext(seelog.InfoLvl, func() { Infoc(message, context...) }, logger.info, message, context...)
}

// Warn logs at the warn level and returns an error containing the formated log message
func Warn(v ...interface{}) error {
	return logWithError(seelog.WarnLvl, func() { Warn(v...) }, logger.warn, false, v...)
}

// Warnf logs with format at the warn level and returns an error containing the formated log message
func Warnf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.WarnLvl, func() { Warnf(format, params...) }, logger.warnf, format, false, params...)
}

// Error logs at the error level and returns an error containing the formated log message
func Error(v ...interface{}) error {
	return logWithError(seelog.ErrorLvl, func() { Error(v...) }, logger.error, true, v...)
}

// Errorf logs with format at the error level and returns an error containing the formated log message
func Errorf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.ErrorLvl, func() { Errorf(format, params...) }, logger.errorf, format, true, params...)
}

// Critical logs at the critical level and returns an error containing the formated log message
func Critical(v ...interface{}) error {
	return logWithError(seelog.CriticalLvl, func() { Critical(v...) }, logger.critical, true, v...)
}

// Criticalf logs with format at the critical level and returns an error containing the formated log message
func Criticalf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.CriticalLvl, func() { Criticalf(format, params...) }, logger.criticalf, format, true, params...)
}

// ErrorStackDepth logs at the error level and the current stack depth plus the additional given one and returns an error containing the formated log message
func ErrorStackDepth(depth int, v ...interface{}) error {
	return logWithError(seelog.ErrorLvl, func() { ErrorStackDepth(depth, v...) }, func(s string) error {
		return logger.errorStackDepth(s, depth)
	}, true, v...)
}

// Flush flushes the underlying inner log
func Flush() {
	if logger != nil && logger.inner != nil {
		logger.inner.Flush()
	}
}

// ReplaceLogger allows replacing the internal logger, returns old logger
func ReplaceLogger(l seelog.LoggerInterface) seelog.LoggerInterface {
	if logger != nil && logger.inner != nil {
		return logger.replaceInnerLogger(l)
	}

	return nil
}

// RegisterAdditionalLogger registers an additional logger for logging
func RegisterAdditionalLogger(n string, l seelog.LoggerInterface) error {
	if logger != nil && logger.inner != nil {
		return logger.registerAdditionalLogger(n, l)
	}

	return errors.New("cannot register: logger not initialized")
}

// UnregisterAdditionalLogger unregisters additional logger with name n
func UnregisterAdditionalLogger(n string) error {
	if logger != nil && logger.inner != nil {
		return logger.unregisterAdditionalLogger(n)
	}

	return errors.New("cannot unregister: logger not initialized")
}

// GetLogLevel returns a seelog native representation of the current
// log level
func GetLogLevel() (seelog.LogLevel, error) {
	if logger != nil && logger.inner != nil {
		return logger.getLogLevel(), nil
	}

	// need to return something, just set to Info (expected default)
	return seelog.InfoLvl, errors.New("cannot get loglevel: logger not initialized")
}

// ChangeLogLevel changes the current log level, valide levels are trace, debug,
// info, warn, error, critical and off, it requires a new seelog logger because
// an existing one cannot be updated
func ChangeLogLevel(l seelog.LoggerInterface, level string) error {
	if logger != nil && logger.inner != nil {
		err := logger.changeLogLevel(level)
		if err != nil {
			return err
		}
		// See detailed explanation in SetupDatadogLogger(...)
		err = l.SetAdditionalStackDepth(defaultStackDepth)
		if err != nil {
			return err
		}

		logger.replaceInnerLogger(l)
		return nil
	}
	// need to return something, just set to Info (expected default)
	return errors.New("cannot change loglevel: logger not initialized")
}
