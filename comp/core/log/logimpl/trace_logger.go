// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logimpl

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfiglogs "github.com/DataDog/datadog-agent/pkg/config/logs"
	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const stackDepth = 3

// tracelogger implements the component
type tracelogger struct {
	// this component is currently implementing a thin wrapper around
	// pkg/trace/log, and uses globals in that package.
}

// Until the log migration to component is done, we use *StackDepth to log. The log component add 1 layer to the call
// stack and *StackDepth add another.
//
// We check the current log level to avoid calling Sprintf when it's not needed (Sprintf from Tracef uses a lot a CPU)

// Trace implements Component#Trace.
func (*tracelogger) Trace(v ...interface{}) { pkglog.TraceStackDepth(stackDepth, v...) }

// Tracef implements Component#Tracef.
func (*tracelogger) Tracef(format string, params ...interface{}) {
	pkglog.TracefStackDepth(stackDepth, format, params...)
}

// Debug implements Component#Debug.
func (*tracelogger) Debug(v ...interface{}) { pkglog.DebugStackDepth(stackDepth, v...) }

// Debugf implements Component#Debugf.
func (*tracelogger) Debugf(format string, params ...interface{}) {
	pkglog.DebugfStackDepth(stackDepth, format, params...)
}

// Info implements Component#Info.
func (*tracelogger) Info(v ...interface{}) { pkglog.InfoStackDepth(stackDepth, v...) }

// Infof implements Component#Infof.
func (*tracelogger) Infof(format string, params ...interface{}) {
	pkglog.InfofStackDepth(stackDepth, format, params...)
}

// Warn implements Component#Warn.
func (*tracelogger) Warn(v ...interface{}) error { return pkglog.WarnStackDepth(stackDepth, v...) }

// Warnf implements Component#Warnf.
func (*tracelogger) Warnf(format string, params ...interface{}) error {
	return pkglog.WarnfStackDepth(stackDepth, format, params...)
}

// Error implements Component#Error.
func (*tracelogger) Error(v ...interface{}) error { return pkglog.ErrorStackDepth(stackDepth, v...) }

// Errorf implements Component#Errorf.
func (*tracelogger) Errorf(format string, params ...interface{}) error {
	return pkglog.ErrorfStackDepth(stackDepth, format, params...)
}

// Critical implements Component#Critical.
func (*tracelogger) Critical(v ...interface{}) error {
	return pkglog.CriticalStackDepth(stackDepth, v...)
}

// Criticalf implements Component#Criticalf.
func (*tracelogger) Criticalf(format string, params ...interface{}) error {
	return pkglog.CriticalfStackDepth(stackDepth, format, params...)
}

// Flush implements Component#Flush.
func (*tracelogger) Flush() { pkglog.Flush() }

func newTraceLogger(lc fx.Lifecycle, params Params, config config.Component, telemetryCollector telemetry.TelemetryCollector) (log.Component, error) {
	return NewTraceLogger(lc, params, config, telemetryCollector)
}

// NewTraceLogger creates a pkglog.Component using the provided config.LogConfig
func NewTraceLogger(lc fx.Lifecycle, params Params, config config.LogConfig, telemetryCollector telemetry.TelemetryCollector) (log.Component, error) {
	if params.logLevelFn == nil {
		return nil, errors.New("must call one of core.BundleParams.ForOneShot or ForDaemon")
	}

	err := pkgconfiglogs.SetupLogger(
		pkgconfiglogs.LoggerName(params.loggerName),
		params.logLevelFn(config),
		params.logFileFn(config),
		params.logSyslogURIFn(config),
		params.logSyslogRFCFn(config),
		params.logToConsoleFn(config),
		params.logFormatJSONFn(config),
		config)
	if err != nil {
		telemetryCollector.SendStartupError(telemetry.CantCreateLogger, err)
		return nil, fmt.Errorf("Cannot create logger: %v", err)
	}

	l := &tracelogger{}
	tracelog.SetLogger(l)
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		l.Flush()
		return nil
	}})

	return l, nil
}
