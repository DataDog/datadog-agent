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

func prefixedMessage(args []interface{}) string { return Prefix + pkglog.BuildLogEntry(args...) }

func Trace(args ...interface{})                 { pkglog.TraceStackDepth(1, prefixedMessage(args)) }
func Tracef(format string, args ...interface{}) { pkglog.TracefStackDepth(1, Prefix+format, args...) }
func Debug(args ...interface{})                 { pkglog.DebugStackDepth(1, prefixedMessage(args)) }
func Debugf(format string, args ...interface{}) { pkglog.DebugfStackDepth(1, Prefix+format, args...) }
func Info(args ...interface{})                  { pkglog.InfoStackDepth(1, prefixedMessage(args)) }
func Infof(format string, args ...interface{})  { pkglog.InfofStackDepth(1, Prefix+format, args...) }
func Warn(args ...interface{}) error            { return pkglog.WarnStackDepth(1, prefixedMessage(args)) }
func Warnf(format string, args ...interface{}) error {
	return pkglog.WarnfStackDepth(1, Prefix+format, args...)
}
func Error(args ...interface{}) error { return pkglog.ErrorStackDepth(1, prefixedMessage(args)) }
func Errorf(format string, args ...interface{}) error {
	return pkglog.ErrorfStackDepth(1, Prefix+format, args...)
}
func Critical(args ...interface{}) error { return pkglog.CriticalStackDepth(1, prefixedMessage(args)) }
func Criticalf(format string, args ...interface{}) error {
	return pkglog.CriticalfStackDepth(1, Prefix+format, args...)
}
