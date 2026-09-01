// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logging provides the common anomaly-detection log prefix.
package logging

import pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

// Prefix identifies logs emitted by the anomaly-detection subsystem. It includes
// the trailing separator so log producers and consumers share one exact marker.
const Prefix = "[anomalydetection] "

// callerSkip accounts for this package's helper frame and the stack-depth helper
// itself, preserving the original logging call site in formatted output.
const callerSkip = 2

func prefixedMessage(args []interface{}) string { return Prefix + pkglog.BuildLogEntry(args...) }

func Trace(args ...interface{}) { pkglog.TraceStackDepth(callerSkip, prefixedMessage(args)) }
func Tracef(format string, args ...interface{}) {
	pkglog.TracefStackDepth(callerSkip, Prefix+format, args...)
}
func Debug(args ...interface{}) { pkglog.DebugStackDepth(callerSkip, prefixedMessage(args)) }
func Debugf(format string, args ...interface{}) {
	pkglog.DebugfStackDepth(callerSkip, Prefix+format, args...)
}
func Info(args ...interface{}) { pkglog.InfoStackDepth(callerSkip, prefixedMessage(args)) }
func Infof(format string, args ...interface{}) {
	pkglog.InfofStackDepth(callerSkip, Prefix+format, args...)
}
func Warn(args ...interface{}) {
	_ = pkglog.WarnStackDepth(callerSkip, prefixedMessage(args))
}
func Warnf(format string, args ...interface{}) {
	_ = pkglog.WarnfStackDepth(callerSkip, Prefix+format, args...)
}
func Error(args ...interface{}) {
	_ = pkglog.ErrorStackDepth(callerSkip, prefixedMessage(args))
}
func Errorf(format string, args ...interface{}) {
	_ = pkglog.ErrorfStackDepth(callerSkip, Prefix+format, args...)
}
func Critical(args ...interface{}) {
	_ = pkglog.CriticalStackDepth(callerSkip, prefixedMessage(args))
}
func Criticalf(format string, args ...interface{}) {
	_ = pkglog.CriticalfStackDepth(callerSkip, Prefix+format, args...)
}
