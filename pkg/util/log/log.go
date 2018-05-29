// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package log

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/cihub/seelog"
)

var logger *DatadogLogger

//DatadogLogger wrapper structure for seelog
type DatadogLogger struct {
	inner seelog.LoggerInterface
	extra map[string]seelog.LoggerInterface
	l     sync.Mutex
}

//SetupDatadogLogger configure logger singleton with seelog interface
func SetupDatadogLogger(l seelog.LoggerInterface) {
	logger = &DatadogLogger{
		inner: l,
		extra: make(map[string]seelog.LoggerInterface),
	}

	//We're not going to call DatadogLogger directly, but using the
	//exported functions, that will give us two frames in the stack
	//trace that should be skipped to get to the original caller.
	//
	//The fact we need a constant "additional depth" means some
	//theoretical refactor to avoid duplication in the functions
	//below cannot be performed.
	logger.inner.SetAdditionalStackDepth(2)
}

func (sw *DatadogLogger) replaceInnerLogger(l seelog.LoggerInterface) seelog.LoggerInterface {
	sw.l.Lock()
	defer sw.l.Unlock()

	old := sw.inner
	sw.inner = l

	return old
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

//trace logs at the trace level
func (sw *DatadogLogger) trace(s string) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.Trace(scrubbed)

	for _, l := range sw.extra {
		l.Trace(scrubbed)
	}
}

//debug logs at the debug level
func (sw *DatadogLogger) debug(s string) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.Debug(scrubbed)

	for _, l := range sw.extra {
		l.Debug(scrubbed)
	}
}

//info logs at the info level
func (sw *DatadogLogger) info(s string) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	sw.inner.Info(scrubbed)

	for _, l := range sw.extra {
		l.Info(scrubbed)
	}
}

//warn logs at the warn level
func (sw *DatadogLogger) warn(s string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	err := sw.inner.Warn(scrubbed)

	for _, l := range sw.extra {
		l.Warn(scrubbed)
	}

	return err
}

//error logs at the error level
func (sw *DatadogLogger) error(s string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	err := sw.inner.Error(scrubbed)

	for _, l := range sw.extra {
		l.Error(scrubbed)
	}

	return err
}

//critical logs at the critical level
func (sw *DatadogLogger) critical(s string) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(s)
	err := sw.inner.Critical(scrubbed)

	for _, l := range sw.extra {
		l.Critical(scrubbed)
	}

	return err
}

//tracef logs with format at the trace level
func (sw *DatadogLogger) tracef(format string, params ...interface{}) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	sw.inner.Trace(scrubbed)

	for _, l := range sw.extra {
		l.Trace(scrubbed)
	}
}

//debugf logs with format at the debug level
func (sw *DatadogLogger) debugf(format string, params ...interface{}) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	sw.inner.Debug(scrubbed)

	for _, l := range sw.extra {
		l.Debug(scrubbed)
	}
}

//infof logs with format at the info level
func (sw *DatadogLogger) infof(format string, params ...interface{}) {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	sw.inner.Info(scrubbed)

	for _, l := range sw.extra {
		l.Info(scrubbed)
	}
}

//warnf logs with format at the warn level
func (sw *DatadogLogger) warnf(format string, params ...interface{}) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	err := sw.inner.Warn(scrubbed)

	for _, l := range sw.extra {
		l.Warn(scrubbed)
	}

	return err
}

//errorf logs with format at the error level
func (sw *DatadogLogger) errorf(format string, params ...interface{}) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	err := sw.inner.Error(scrubbed)

	for _, l := range sw.extra {
		l.Error(scrubbed)
	}

	return err
}

//criticalf logs with format at the critical level
func (sw *DatadogLogger) criticalf(format string, params ...interface{}) error {
	sw.l.Lock()
	defer sw.l.Unlock()

	scrubbed := sw.scrub(fmt.Sprintf(format, params...))
	err := sw.inner.Critical(scrubbed)

	for _, l := range sw.extra {
		l.Critical(scrubbed)
	}

	return err
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

//ReplaceLogger allows replacing the internal logger, returns old logger
func ReplaceLogger(l seelog.LoggerInterface) seelog.LoggerInterface {
	if logger != nil && logger.inner != nil {
		return logger.replaceInnerLogger(l)
	}

	return nil
}

//RegisterAdditionalLogger registers an additional logger for logging
func RegisterAdditionalLogger(n string, l seelog.LoggerInterface) error {
	if logger != nil && logger.inner != nil {
		return logger.registerAdditionalLogger(n, l)
	}

	return errors.New("cannot register: logger not initialized")
}

//UnregisterAdditionalLogger unregisters additional logger with name n
func UnregisterAdditionalLogger(n string) error {
	if logger != nil && logger.inner != nil {
		return logger.unregisterAdditionalLogger(n)
	}

	return errors.New("cannot unregister: logger not initialized")
}
