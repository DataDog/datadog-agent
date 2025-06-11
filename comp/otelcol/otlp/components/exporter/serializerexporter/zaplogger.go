// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"fmt"

	"go.uber.org/zap"

	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
)

var _ tracelog.Logger = &ZapLoggerOtel{}

// ZapLoggerOtel implements the tracelog.Logger interface on top of a zap.Logger
// It is a duplicate of github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/zaplogger.go
// It is used to remove the dependency on the opentelemetry-collector-contrib package.
type ZapLoggerOtel struct {
	// Logger is the internal zap logger
	Logger *zap.Logger
}

// Trace implements Logger.
func (z *ZapLoggerOtel) Trace(_ ...any) { /* N/A */ }

// Tracef implements Logger.
func (z *ZapLoggerOtel) Tracef(_ string, _ ...any) { /* N/A */ }

// Debug implements Logger.
func (z *ZapLoggerOtel) Debug(v ...any) {
	z.Logger.Debug(fmt.Sprint(v...))
}

// Debugf implements Logger.
func (z *ZapLoggerOtel) Debugf(format string, params ...any) {
	z.Logger.Debug(fmt.Sprintf(format, params...))
}

// Info implements Logger.
func (z *ZapLoggerOtel) Info(v ...any) {
	z.Logger.Info(fmt.Sprint(v...))
}

// Infof implements Logger.
func (z *ZapLoggerOtel) Infof(format string, params ...any) {
	z.Logger.Info(fmt.Sprintf(format, params...))
}

// Warn implements Logger.
func (z *ZapLoggerOtel) Warn(v ...any) error {
	z.Logger.Warn(fmt.Sprint(v...))
	return nil
}

// Warnf implements Logger.
func (z *ZapLoggerOtel) Warnf(format string, params ...any) error {
	z.Logger.Warn(fmt.Sprintf(format, params...))
	return nil
}

// Error implements Logger.
func (z *ZapLoggerOtel) Error(v ...any) error {
	z.Logger.Error(fmt.Sprint(v...))
	return nil
}

// Errorf implements Logger.
func (z *ZapLoggerOtel) Errorf(format string, params ...any) error {
	z.Logger.Error(fmt.Sprintf(format, params...))
	return nil
}

// Critical implements Logger.
func (z *ZapLoggerOtel) Critical(v ...any) error {
	z.Logger.Error(fmt.Sprint(v...), zap.Bool("critical", true))
	return nil
}

// Criticalf implements Logger.
func (z *ZapLoggerOtel) Criticalf(format string, params ...any) error {
	z.Logger.Error(fmt.Sprintf(format, params...), zap.Bool("critical", true))
	return nil
}

// Flush implements Logger.
func (z *ZapLoggerOtel) Flush() {
	_ = z.Logger.Sync()
}
