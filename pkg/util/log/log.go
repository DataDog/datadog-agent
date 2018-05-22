// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package log

import (
	"bytes"
	"fmt"

	"github.com/cihub/seelog"
)

var logger *DatadogLogger

//DatadogLogger wrapper structure for seelog
type DatadogLogger struct {
	inner seelog.LoggerInterface
}

//SetupDatadogLogger configure logger singleton with seelog interface
func SetupDatadogLogger(l seelog.LoggerInterface) {
	logger = &DatadogLogger{
		inner: l,
	}

	//We're not going to call DatadogLogger directly, but using the
	//exported functions, that will give us two frames in the stack
	//trace that should be skipped to get to the original caller.
	logger.inner.SetAdditionalStackDepth(2)
}

func (sw *DatadogLogger) scrub(s string) string {
	if scrubbed, err := CredentialsCleanerBytes([]byte(s)); err == nil {
		return string(scrubbed)
	}

	return s
}

//trace logs at the trace level
func (sw *DatadogLogger) trace(s string) {
	sw.inner.Trace(sw.scrub(s))
}

//debug logs at the debug level
func (sw *DatadogLogger) debug(s string) {
	sw.inner.Debug(sw.scrub(s))
}

//info logs at the info level
func (sw *DatadogLogger) info(s string) {
	sw.inner.Info(sw.scrub(s))
}

//warn logs at the warn level
func (sw *DatadogLogger) warn(s string) error {
	return sw.inner.Warn(sw.scrub(s))
}

//error logs at the error level
func (sw *DatadogLogger) error(s string) error {
	return sw.inner.Error(sw.scrub(s))
}

//critical logs at the critical level
func (sw *DatadogLogger) critical(s string) error {
	return sw.inner.Critical(sw.scrub(s))
}

//tracef logs with format at the trace level
func (sw *DatadogLogger) tracef(format string, params ...interface{}) {
	sw.inner.Trace(sw.scrub(fmt.Sprintf(format, params...)))
}

//debugf logs with format at the debug level
func (sw *DatadogLogger) debugf(format string, params ...interface{}) {
	sw.inner.Debug(sw.scrub(fmt.Sprintf(format, params...)))
}

//infof logs with format at the info level
func (sw *DatadogLogger) infof(format string, params ...interface{}) {
	sw.inner.Info(sw.scrub(fmt.Sprintf(format, params...)))
}

//warnf logs with format at the warn level
func (sw *DatadogLogger) warnf(format string, params ...interface{}) error {
	return sw.inner.Warn(sw.scrub(fmt.Sprintf(format, params...)))
}

//errorf logs with format at the error level
func (sw *DatadogLogger) errorf(format string, params ...interface{}) error {
	return sw.inner.Error(sw.scrub(fmt.Sprintf(format, params...)))
}

//criticalf logs with format at the critical level
func (sw *DatadogLogger) criticalf(format string, params ...interface{}) error {
	return sw.inner.Critical(sw.scrub(fmt.Sprintf(format, params...)))
}

func buildLogEntry(v ...interface{}) string {
	var fmtBuffer bytes.Buffer

	for i := 0; i < len(v)-1; i++ {
		fmtBuffer.WriteString("%v ")
	}
	fmtBuffer.WriteString("%v")

	return fmt.Sprintf(fmtBuffer.String(), v...)
}

//Trace logs at the trace level
func Trace(v ...interface{}) {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		logger.trace(logger.scrub(s))
	}
}

//Debug logs at the debug level
func Debug(v ...interface{}) {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		logger.debug(logger.scrub(s))
	}
}

//Info logs at the info level
func Info(v ...interface{}) {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		logger.info(logger.scrub(s))
	}
}

//Warn logs at the warn level
func Warn(v ...interface{}) error {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		return logger.warn(logger.scrub(s))
	}
	return nil
}

//Error logs at the error level
func Error(v ...interface{}) error {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		return logger.error(logger.scrub(s))
	}
	return nil
}

//Critical logs at the critical level
func Critical(v ...interface{}) error {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		return logger.critical(logger.scrub(s))
	}
	return nil
}

//Flush flushes the underlying inner log
func Flush() {
	if logger != nil && logger.inner != nil {
		logger.inner.Flush()
	}
}

//Tracef logs with format at the trace level
func Tracef(format string, params ...interface{}) {
	if logger != nil && logger.inner != nil {
		logger.tracef(format, params...)
	}
}

//Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	if logger != nil && logger.inner != nil {
		logger.debugf(format, params...)
	}
}

//Infof logs with format at the info level
func Infof(format string, params ...interface{}) {
	if logger != nil && logger.inner != nil {
		logger.infof(format, params...)
	}
}

//Warnf logs with format at the warn level
func Warnf(format string, params ...interface{}) error {
	if logger != nil && logger.inner != nil {
		return logger.warnf(format, params...)
	}
	return nil
}

//Errorf logs with format at the error level
func Errorf(format string, params ...interface{}) error {
	if logger != nil && logger.inner != nil {
		return logger.errorf(format, params...)
	}
	return nil
}

//Criticalf logs with format at the critical level
func Criticalf(format string, params ...interface{}) error {
	if logger != nil && logger.inner != nil {
		return logger.criticalf(format, params...)
	}
	return nil
}
