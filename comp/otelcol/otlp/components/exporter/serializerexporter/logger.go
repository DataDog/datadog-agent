// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"fmt"

	"go.uber.org/zap"
)

// zaplogger implements the tracelog.Logger interface on top of a zap.Logger
type zaplogger struct{ logger *zap.Logger }

// Trace implements Logger.
func (z *zaplogger) Trace(_ ...any) { /* N/A */ }

// Tracef implements Logger.
func (z *zaplogger) Tracef(_ string, _ ...any) { /* N/A */ }

// Debug implements Logger.
func (z *zaplogger) Debug(v ...any) {
	z.logger.Debug(fmt.Sprint(v...))
}

// Debugf implements Logger.
func (z *zaplogger) Debugf(format string, params ...any) {
	z.logger.Debug(fmt.Sprintf(format, params...))
}

// Info implements Logger.
func (z *zaplogger) Info(v ...any) {
	z.logger.Info(fmt.Sprint(v...))
}

// Infof implements Logger.
func (z *zaplogger) Infof(format string, params ...any) {
	z.logger.Info(fmt.Sprintf(format, params...))
}

// Warn implements Logger.
func (z *zaplogger) Warn(v ...any) error {
	z.logger.Warn(fmt.Sprint(v...))
	return nil
}

// Warnf implements Logger.
func (z *zaplogger) Warnf(format string, params ...any) error {
	z.logger.Warn(fmt.Sprintf(format, params...))
	return nil
}

// Error implements Logger.
func (z *zaplogger) Error(v ...any) error {
	z.logger.Error(fmt.Sprint(v...))
	return nil
}

// Errorf implements Logger.
func (z *zaplogger) Errorf(format string, params ...any) error {
	z.logger.Error(fmt.Sprintf(format, params...))
	return nil
}

// Critical implements Logger.
func (z *zaplogger) Critical(v ...any) error {
	z.logger.Error(fmt.Sprint(v...), zap.Bool("critical", true))
	return nil
}

// Criticalf implements Logger.
func (z *zaplogger) Criticalf(format string, params ...any) error {
	z.logger.Error(fmt.Sprintf(format, params...), zap.Bool("critical", true))
	return nil
}

// Flush implements Logger.
func (z *zaplogger) Flush() {
	_ = z.logger.Sync()
}
