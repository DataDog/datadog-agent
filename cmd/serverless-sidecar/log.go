// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// nolint
package main

import "github.com/DataDog/datadog-agent/pkg/util/log"

// Stack depth of 3 since the `corelogger` struct adds a layer above the logger
const stackDepth = 3

type corelogger struct{}

// Trace implements Logger.
func (corelogger) Trace(v ...interface{}) { log.TraceStackDepth(stackDepth, v...) }

// Tracef implements Logger.
func (corelogger) Tracef(format string, params ...interface{}) {
	log.TracefStackDepth(stackDepth, format, params...)
}

// Debug implements Logger.
func (corelogger) Debug(v ...interface{}) { log.DebugStackDepth(stackDepth, v...) }

// Debugf implements Logger.
func (corelogger) Debugf(format string, params ...interface{}) {
	log.DebugfStackDepth(stackDepth, format, params...)
}

// Info implements Logger.
func (corelogger) Info(v ...interface{}) { log.InfoStackDepth(stackDepth, v...) }

// Infof implements Logger.
func (corelogger) Infof(format string, params ...interface{}) {
	log.InfofStackDepth(stackDepth, format, params...)
}

// Warn implements Logger.
func (corelogger) Warn(v ...interface{}) error { return log.WarnStackDepth(stackDepth, v...) }

// Warnf implements Logger.
func (corelogger) Warnf(format string, params ...interface{}) error {
	return log.WarnfStackDepth(stackDepth, format, params...)
}

// Error implements Logger.
func (corelogger) Error(v ...interface{}) error { return log.ErrorStackDepth(stackDepth, v...) }

// Errorf implements Logger.
func (corelogger) Errorf(format string, params ...interface{}) error {
	return log.ErrorfStackDepth(stackDepth, format, params...)
}

// Critical implements Logger.
func (corelogger) Critical(v ...interface{}) error { return log.CriticalStackDepth(stackDepth, v...) }

// Criticalf implements Logger.
func (corelogger) Criticalf(format string, params ...interface{}) error {
	return log.CriticalfStackDepth(stackDepth, format, params...)
}

// Flush implements Logger.
func (corelogger) Flush() { log.Flush() }
