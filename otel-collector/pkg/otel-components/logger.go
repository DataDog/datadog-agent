package otelcomponents

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"go.uber.org/zap"
)

// Zaplogger implements the tracelog.Logger interface on top of a zap.Logger
type Zaplogger struct{ logger *zap.Logger }

var _ log.Component = &Zaplogger{}

// Trace implements Logger.
func (z *Zaplogger) Trace(v ...interface{}) { /* N/A */ }

// Tracef implements Logger.
func (z *Zaplogger) Tracef(format string, params ...interface{}) { /* N/A */ }

// Debug implements Logger.
func (z *Zaplogger) Debug(v ...interface{}) {
	z.logger.Debug(fmt.Sprint(v...))
}

// Debugf implements Logger.
func (z *Zaplogger) Debugf(format string, params ...interface{}) {
	z.logger.Debug(fmt.Sprintf(format, params...))
}

// Info implements Logger.
func (z *Zaplogger) Info(v ...interface{}) {
	z.logger.Info(fmt.Sprint(v...))
}

// Infof implements Logger.
func (z *Zaplogger) Infof(format string, params ...interface{}) {
	z.logger.Info(fmt.Sprintf(format, params...))
}

// Warn implements Logger.
func (z *Zaplogger) Warn(v ...interface{}) error {
	z.logger.Warn(fmt.Sprint(v...))
	return nil
}

// Warnf implements Logger.
func (z *Zaplogger) Warnf(format string, params ...interface{}) error {
	z.logger.Warn(fmt.Sprintf(format, params...))
	return nil
}

// Error implements Logger.
func (z *Zaplogger) Error(v ...interface{}) error {
	z.logger.Error(fmt.Sprint(v...))
	return nil
}

// Errorf implements Logger.
func (z *Zaplogger) Errorf(format string, params ...interface{}) error {
	z.logger.Error(fmt.Sprintf(format, params...))
	return nil
}

// Critical implements Logger.
func (z *Zaplogger) Critical(v ...interface{}) error {
	z.logger.Error(fmt.Sprint(v...), zap.Bool("critical", true))
	return nil
}

// Criticalf implements Logger.
func (z *Zaplogger) Criticalf(format string, params ...interface{}) error {
	z.logger.Error(fmt.Sprintf(format, params...), zap.Bool("critical", true))
	return nil
}

// Flush implements Logger.
func (z *Zaplogger) Flush() {
	_ = z.logger.Sync()
}
