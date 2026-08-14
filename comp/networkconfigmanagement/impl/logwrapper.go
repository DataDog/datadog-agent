// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

type LogWrapper struct {
	prefix string
	logger log.Component
}

var _ log.Component = (*LogWrapper)(nil)

// NewLogWrapper returns a log.Component that prepends prefix to every log message.
func NewLogWrapper(logger log.Component, prefix string) *LogWrapper {
	return &LogWrapper{logger: logger, prefix: prefix}
}

func (l *LogWrapper) Trace(v ...any) {
	l.logger.Trace(append([]any{l.prefix}, v...)...)
}

func (l *LogWrapper) Tracef(format string, params ...any) {
	l.logger.Tracef(l.prefix+format, params...)
}

func (l *LogWrapper) Debug(v ...any) {
	l.logger.Debug(append([]any{l.prefix}, v...)...)
}

func (l *LogWrapper) Debugf(format string, params ...any) {
	l.logger.Debugf(l.prefix+format, params...)
}

func (l *LogWrapper) Info(v ...any) {
	l.logger.Info(append([]any{l.prefix}, v...)...)
}

func (l *LogWrapper) Infof(format string, params ...any) {
	l.logger.Infof(l.prefix+format, params...)
}

func (l *LogWrapper) Warn(v ...any) error {
	return l.logger.Warn(append([]any{l.prefix}, v...)...)
}

func (l *LogWrapper) Warnf(format string, params ...any) error {
	return l.logger.Warnf(l.prefix+format, params...)
}

func (l *LogWrapper) Error(v ...any) error {
	return l.logger.Error(append([]any{l.prefix}, v...)...)
}

func (l *LogWrapper) Errorf(format string, params ...any) error {
	return l.logger.Errorf(l.prefix+format, params...)
}

func (l *LogWrapper) Critical(v ...any) error {
	return l.logger.Critical(append([]any{l.prefix}, v...)...)
}

func (l *LogWrapper) Criticalf(format string, params ...any) error {
	return l.logger.Criticalf(l.prefix+format, params...)
}

func (l *LogWrapper) Flush() {
	l.logger.Flush()
}

// NoopLogger is a log.Component that discards all messages.
type NoopLogger struct{}

var _ log.Component = NoopLogger{}

func (NoopLogger) Trace(...any)                   {}
func (NoopLogger) Tracef(string, ...any)          {}
func (NoopLogger) Debug(...any)                   {}
func (NoopLogger) Debugf(string, ...any)          {}
func (NoopLogger) Info(...any)                    {}
func (NoopLogger) Infof(string, ...any)           {}
func (NoopLogger) Warn(...any) error              { return nil }
func (NoopLogger) Warnf(string, ...any) error     { return nil }
func (NoopLogger) Error(...any) error             { return nil }
func (NoopLogger) Errorf(string, ...any) error    { return nil }
func (NoopLogger) Critical(...any) error          { return nil }
func (NoopLogger) Criticalf(string, ...any) error { return nil }
func (NoopLogger) Flush()                         {}

type logContextKey struct{}

// WithLogger returns a new context carrying logger.
func WithLogger(ctx context.Context, logger log.Component) context.Context {
	return context.WithValue(ctx, logContextKey{}, logger)
}

// LoggerFromContext returns the logger stored in ctx, or a NoopLogger if none.
func LoggerFromContext(ctx context.Context) log.Component {
	if l, ok := ctx.Value(logContextKey{}).(log.Component); ok && l != nil {
		return l
	}
	return NoopLogger{}
}
