// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package log

import (
	"github.com/cihub/seelog"
)

var log *DatadogLogger

type DatadogLogger struct {
	inner seelog.LoggerInterface
}

func SetupDatadogLogger(l seelog.LoggerInterface) {
	log = &DatadogLogger{
		inner: l,
	}
}

func (sw *DatadogLogger) scrub(s string) string {
	if scrubbed, err = flare.CredentialsCleanerBytes([]byte(s)); err == nil {
		return string(scrubbed)
	}

	return s
}
func (sw *DatadogLogger) Trace(s string) {
	sw.inner.Trace(sw.scrub(s))
}
func (sw *DatadogLogger) Debug(s string) {
	sw.inner.Debug(sw.scrub(s))
}
func (sw *DatadogLogger) Info(s string) {
	sw.inner.Info(sw.scrub(s))
}
func (sw *DatadogLogger) Warn(s string) {
	sw.inner.Warn(sw.scrub(s))
}
func (sw *DatadogLogger) Error(s string) {
	sw.inner.Error(sw.scrub(s))
}
func (sw *DatadogLogger) Critical(s string) {
	sw.inner.Critical(sw.scrub(s))
}
func (sw *DatadogLogger) Fatal(s string) {
	sw.inner.Fatal(sw.scrub(s))
}
func (sw *DatadogLogger) Tracef(format string, params ...interface{}) {
	sw.inner.Trace(fmt.Sprintf(format, params))
}
func (sw *DatadogLogger) Debugf(format string, params ...interface{}) {
	sw.inner.Debug(fmt.Sprintf(format, params))
}
func (sw *DatadogLogger) Infof(format string, params ...interface{}) {
	sw.inner.Info(fmt.Sprintf(format, params))
}
func (sw *DatadogLogger) Warnf(format string, params ...interface{}) {
	sw.inner.Warn(fmt.Sprintf(format, params))
}
func (sw *DatadogLogger) Errorf(format string, params ...interface{}) {
	sw.inner.Error(fmt.Sprintf(format, params))
}
func (sw *DatadogLogger) Criticalf(format string, params ...interface{}) {
	sw.inner.Critical(fmt.Sprintf(format, params))
}
func (sw *DatadogLogger) Fatalf(format string, params ...interface{}) {
	sw.inner.Fatal(fmt.Sprintf(format, params))
}

func Trace(s string) {
	if log != nil && log.inner != nil {
		log.Trace(sw.scrub(s))
	}
}
func Debug(s string) {
	if log != nil && log.inner != nil {
		log.Debug(sw.scrub(s))
	}
}
func Info(s string) {
	if log != nil && log.inner != nil {
		log.Info(sw.scrub(s))
	}
}
func Warn(s string) {
	if log != nil && log.inner != nil {
		log.Warn(sw.scrub(s))
	}
}
func Error(s string) {
	if log != nil && log.inner != nil {
		log.Error(sw.scrub(s))
	}
}
func Critical(s string) {
	if log != nil && log.inner != nil {
		log.Critical(sw.scrub(s))
	}
}
func Fatal(s string) {
	if log != nil && log.inner != nil {
		log.Fatal(sw.scrub(s))
	}
}
func Tracef(format string, params ...interface{}) {
	log.Trace(fmt.Sprintf(format, params))
}
func Debugf(format string, params ...interface{}) {
	log.Debug(fmt.Sprintf(format, params))
}
func Infof(format string, params ...interface{}) {
	log.Info(fmt.Sprintf(format, params))
}
func Warnf(format string, params ...interface{}) {
	log.Warn(fmt.Sprintf(format, params))
}
func Errorf(format string, params ...interface{}) {
	log.Error(fmt.Sprintf(format, params))
}
func Criticalf(format string, params ...interface{}) {
	log.Critical(fmt.Sprintf(format, params))
}
func Fatalf(format string, params ...interface{}) {
	log.Fatal(fmt.Sprintf(format, params))
}
