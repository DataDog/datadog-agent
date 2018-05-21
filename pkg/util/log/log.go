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
}

func (sw *DatadogLogger) scrub(s string) string {
	if scrubbed, err := CredentialsCleanerBytes([]byte(s)); err == nil {
		return string(scrubbed)
	}

	return s
}

//Trace logs at the trace level
func (sw *DatadogLogger) Trace(s string) {
	sw.inner.Trace(sw.scrub(s))
}

//Debug logs at the debug level
func (sw *DatadogLogger) Debug(s string) {
	sw.inner.Debug(sw.scrub(s))
}

//Info logs at the info level
func (sw *DatadogLogger) Info(s string) {
	sw.inner.Info(sw.scrub(s))
}

//Warn logs at the warn level
func (sw *DatadogLogger) Warn(s string) error {
	return sw.inner.Warn(sw.scrub(s))
}

//Error logs at the error level
func (sw *DatadogLogger) Error(s string) error {
	return sw.inner.Error(sw.scrub(s))
}

//Critical logs at the critical level
func (sw *DatadogLogger) Critical(s string) error {
	return sw.inner.Critical(sw.scrub(s))
}

//Tracef logs with format at the trace level
func (sw *DatadogLogger) Tracef(format string, params ...interface{}) {
	sw.inner.Trace(fmt.Sprintf(format, params))
}

//Debugf logs with format at the debug level
func (sw *DatadogLogger) Debugf(format string, params ...interface{}) {
	sw.inner.Debug(fmt.Sprintf(format, params))
}

//Infof logs with format at the info level
func (sw *DatadogLogger) Infof(format string, params ...interface{}) {
	sw.inner.Info(fmt.Sprintf(format, params))
}

//Warnf logs with format at the warn level
func (sw *DatadogLogger) Warnf(format string, params ...interface{}) error {
	return sw.inner.Warn(fmt.Sprintf(format, params))
}

//Errorf logs with format at the error level
func (sw *DatadogLogger) Errorf(format string, params ...interface{}) error {
	return sw.inner.Error(fmt.Sprintf(format, params))
}

//Criticalf logs with format at the critical level
func (sw *DatadogLogger) Criticalf(format string, params ...interface{}) error {
	return sw.inner.Critical(fmt.Sprintf(format, params))
}

func buildLogEntry(v ...interface{}) string {
	var fmtBuffer bytes.Buffer
	for i := 0; i < len(v)-1; i++ {
		fmtBuffer.WriteString("%s ")
	}
	fmtBuffer.WriteString("%s")

	return fmt.Sprintf(fmtBuffer.String(), v)
}

//Trace logs at the trace level
func Trace(v ...interface{}) {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		logger.Trace(logger.scrub(s))
	}
}

//Debug logs at the debug level
func Debug(v ...interface{}) {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		logger.Debug(logger.scrub(s))
	}
}

//Info logs at the info level
func Info(v ...interface{}) {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		logger.Info(logger.scrub(s))
	}
}

//Warn logs at the warn level
func Warn(v ...interface{}) error {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		return logger.Warn(logger.scrub(s))
	}
	return nil
}

//Error logs at the error level
func Error(v ...interface{}) error {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		return logger.Error(logger.scrub(s))
	}
	return nil
}

//Critical logs at the critical level
func Critical(v ...interface{}) error {
	if logger != nil && logger.inner != nil {
		s := buildLogEntry(v)
		return logger.Critical(logger.scrub(s))
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
	Trace(fmt.Sprintf(format, params))
}

//Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	Debug(fmt.Sprintf(format, params))
}

//Infof logs with format at the info level
func Infof(format string, params ...interface{}) {
	Info(fmt.Sprintf(format, params))
}

//Warnf logs with format at the warn level
func Warnf(format string, params ...interface{}) error {
	return Warn(fmt.Sprintf(format, params))
}

//Errorf logs with format at the error level
func Errorf(format string, params ...interface{}) error {
	return Error(fmt.Sprintf(format, params))
}

//Criticalf logs with format at the critical level
func Criticalf(format string, params ...interface{}) error {
	return Critical(fmt.Sprintf(format, params))
}
