// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logging provides a logger that logs fx events.
package logging

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

// Logger is a logger that logs fx events.
type Logger interface {
	Debug(v ...interface{})
}

type fxEventLogger struct {
	logger func(v ...interface{})
}

// NewFxEventLoggerOption returns an fx option that provides a fxEventLogger.
// Generic is used in order to not depends on the logger package.
func NewFxEventLoggerOption[T Logger]() fx.Option {
	// Note: The pointer in *T is needed for `optional:"true"`
	return fx.Provide(func(logger *T) *fxEventLogger {
		if logger == nil {
			return nil
		}
		return &fxEventLogger{logger: (*logger).Debug}
	})
}

// Write writes the given bytes to the logger.
func (l *fxEventLogger) Write(p []byte) (n int, err error) {
	l.logger(string(p))
	return len(p), nil
}

type loggingParams struct {
	fx.In

	// Note to the reader: Don't use `optional:"true"` except if you truly understand how does it work
	// See https://github.com/uber-go/fx/issues/613. It should ideally use only for logging and debuggin purpose.
	FxEventLogging *fxEventLogger `optional:"true"`
}

// FxLoggingOption returns an fx.Option that provides a logger that logs fx events.
// If fxEventLogger is provided, it will be used, otherwise nothing is logged.
// Typically, this logs fx events when log_level is debug or above.
func FxLoggingOption() fx.Option {
	return fx.WithLogger(
		func(params loggingParams) fxevent.Logger {
			if params.FxEventLogging != nil {
				return &fxevent.ConsoleLogger{W: params.FxEventLogging}
			}
			return fxevent.NopLogger
		},
	)
}
