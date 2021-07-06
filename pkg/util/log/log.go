// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	logger    *DatadogLogger
	jmxLogger *DatadogLogger

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

// SetupLogger setup agent wide logger
func SetupLogger(i seelog.LoggerInterface, level string) {
	logger = setupCommonLogger(i, level)

	// Flush the log entries logged before initialization now that the logger is initialized
	bufferMutex.Lock()
	bufferLogsBeforeInit = false
	defer bufferMutex.Unlock()
	for _, logLine := range logsBuffer {
		logLine()
	}
	logsBuffer = []func(){}
}

// SetupJMXLogger setup JMXfetch specific logger
func SetupJMXLogger(i seelog.LoggerInterface, level string) {
	jmxLogger = setupCommonLogger(i, level)
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

// trace logs at the trace level and the current stack depth plus the additional given one
func (sw *DatadogLogger) traceStackDepth(s string, depth int) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	sw.inner.Trace(scrubbed)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

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

// debug logs at the debug level and the current stack depth plus the additional given one
func (sw *DatadogLogger) debugStackDepth(s string, depth int) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	sw.inner.Debug(scrubbed)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range sw.extra {
		l.Debug(scrubbed) //nolint:errcheck
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

// info logs at the info level and the current stack depth plus the additional given one
func (sw *DatadogLogger) infoStackDepth(s string, depth int) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	sw.inner.Info(scrubbed)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

	for _, l := range sw.extra {
		l.Info(scrubbed) //nolint:errcheck
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

// error logs at the error level and the current stack depth plus the additional given one
func (sw *DatadogLogger) warnStackDepth(s string, depth int) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	err := sw.inner.Warn(scrubbed)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

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

// critical logs at the critical level and the current stack depth plus the additional given one
func (sw *DatadogLogger) criticalStackDepth(s string, depth int) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth + depth) //nolint:errcheck
	err := sw.inner.Critical(scrubbed)
	sw.inner.SetAdditionalStackDepth(defaultStackDepth) //nolint:errcheck

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

func logContextWithError(logLevel seelog.LogLevel, bufferFunc func(), logFunc func(string) error, message string, fallbackStderr bool, context ...interface{}) error {
	if logger != nil && logger.inner != nil && logger.shouldLog(logLevel) {
		msg := logger.scrub(message)
		logger.contextLock.Lock()
		logger.inner.SetContext(context)
		err := logFunc(msg)
		logger.inner.SetContext(nil)
		// Not using defer to make sure we release lock as fast as possible
		logger.contextLock.Unlock()
		return err
	} else if bufferLogsBeforeInit && (logger == nil || logger.inner == nil) {
		addLogToBuffer(bufferFunc)
	}
	err := formatErrorc(message, context...)
	if fallbackStderr {
		fmt.Fprintf(os.Stderr, "%s: %s\n", logLevel.String(), err.Error())
	}
	return err
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

// Warnc logs at the warn level with context and returns an error containing the formated log message
func Warnc(message string, context ...interface{}) error {
	return logContextWithError(seelog.WarnLvl, func() { Warnc(message, context...) }, logger.warn, message, false, context...)
}

// Error logs at the error level and returns an error containing the formated log message
func Error(v ...interface{}) error {
	return logWithError(seelog.ErrorLvl, func() { Error(v...) }, logger.error, true, v...)
}

// Errorf logs with format at the error level and returns an error containing the formated log message
func Errorf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.ErrorLvl, func() { Errorf(format, params...) }, logger.errorf, format, true, params...)
}

// Errorc logs at the error level with context and returns an error containing the formated log message
func Errorc(message string, context ...interface{}) error {
	return logContextWithError(seelog.ErrorLvl, func() { Errorc(message, context...) }, logger.error, message, true, context...)
}

// Critical logs at the critical level and returns an error containing the formated log message
func Critical(v ...interface{}) error {
	return logWithError(seelog.CriticalLvl, func() { Critical(v...) }, logger.critical, true, v...)
}

// Criticalf logs with format at the critical level and returns an error containing the formated log message
func Criticalf(format string, params ...interface{}) error {
	return logFormatWithError(seelog.CriticalLvl, func() { Criticalf(format, params...) }, logger.criticalf, format, true, params...)
}

// Criticalc logs at the critical level with context and returns an error containing the formated log message
func Criticalc(message string, context ...interface{}) error {
	return logContextWithError(seelog.CriticalLvl, func() { Criticalc(message, context...) }, logger.critical, message, true, context...)
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

// JMXError Logs for JMX check
func JMXError(v ...interface{}) error {
	return logWithError(seelog.ErrorLvl, func() { JMXError(v...) }, jmxLogger.error, true, v...)
}

//JMXInfo Logs
func JMXInfo(v ...interface{}) {
	log(seelog.InfoLvl, func() { JMXInfo(v...) }, jmxLogger.info, v...)
}

// Flush flushes the underlying inner log
func Flush() {
	if logger != nil && logger.inner != nil {
		logger.inner.Flush()
	}
	if jmxLogger != nil && jmxLogger.inner != nil {
		jmxLogger.inner.Flush()
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
		// See detailed explanation in SetupLogger(...)
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
