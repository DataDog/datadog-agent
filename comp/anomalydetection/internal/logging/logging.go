// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logging provides the common anomaly-detection log prefix.
package logging

import (
	"fmt"

	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Prefix identifies logs emitted by the anomaly-detection subsystem. It includes
// the trailing separator so log producers and consumers share one exact marker.
const Prefix = "[anomalydetection] "

func prefixedFormat(format string) string { return Prefix + format }

func prefixedMessage(args []interface{}) string { return Prefix + fmt.Sprint(args...) }

// Wrap returns a logger that prefixes every message with Prefix.
func Wrap(logger logdef.Component) logdef.Component {
	if logger == nil {
		return nil
	}
	return wrappedComponent{logger: logger}
}

type wrappedComponent struct{ logger logdef.Component }

func (l wrappedComponent) Trace(args ...interface{}) { l.logger.Trace(prefixedMessage(args)) }
func (l wrappedComponent) Tracef(format string, args ...interface{}) {
	l.logger.Tracef(prefixedFormat(format), args...)
}
func (l wrappedComponent) Debug(args ...interface{}) { l.logger.Debug(prefixedMessage(args)) }
func (l wrappedComponent) Debugf(format string, args ...interface{}) {
	l.logger.Debugf(prefixedFormat(format), args...)
}
func (l wrappedComponent) Info(args ...interface{}) { l.logger.Info(prefixedMessage(args)) }
func (l wrappedComponent) Infof(format string, args ...interface{}) {
	l.logger.Infof(prefixedFormat(format), args...)
}
func (l wrappedComponent) Warn(args ...interface{}) error {
	return l.logger.Warn(prefixedMessage(args))
}
func (l wrappedComponent) Warnf(format string, args ...interface{}) error {
	return l.logger.Warnf(prefixedFormat(format), args...)
}
func (l wrappedComponent) Error(args ...interface{}) error {
	return l.logger.Error(prefixedMessage(args))
}
func (l wrappedComponent) Errorf(format string, args ...interface{}) error {
	return l.logger.Errorf(prefixedFormat(format), args...)
}
func (l wrappedComponent) Critical(args ...interface{}) error {
	return l.logger.Critical(prefixedMessage(args))
}
func (l wrappedComponent) Criticalf(format string, args ...interface{}) error {
	return l.logger.Criticalf(prefixedFormat(format), args...)
}
func (l wrappedComponent) Flush() { l.logger.Flush() }

func Trace(args ...interface{})                 { pkglog.Trace(prefixedMessage(args)) }
func Tracef(format string, args ...interface{}) { pkglog.Tracef(prefixedFormat(format), args...) }
func Debug(args ...interface{})                 { pkglog.Debug(prefixedMessage(args)) }
func Debugf(format string, args ...interface{}) { pkglog.Debugf(prefixedFormat(format), args...) }
func Info(args ...interface{})                  { pkglog.Info(prefixedMessage(args)) }
func Infof(format string, args ...interface{})  { pkglog.Infof(prefixedFormat(format), args...) }
func Warn(args ...interface{}) error            { return pkglog.Warn(prefixedMessage(args)) }
func Warnf(format string, args ...interface{}) error {
	return pkglog.Warnf(prefixedFormat(format), args...)
}
func Error(args ...interface{}) error { return pkglog.Error(prefixedMessage(args)) }
func Errorf(format string, args ...interface{}) error {
	return pkglog.Errorf(prefixedFormat(format), args...)
}
func Critical(args ...interface{}) error { return pkglog.Critical(prefixedMessage(args)) }
func Criticalf(format string, args ...interface{}) error {
	return pkglog.Criticalf(prefixedFormat(format), args...)
}
