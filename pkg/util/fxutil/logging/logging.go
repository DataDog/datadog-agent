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

// FxEventLogger is a logger that logs fx events.
type FxEventLogger struct {
	logger func(v ...interface{})
}

// NewFxEventLogging returns a new fxEventLogging.
func NewFxEventLogging(logger func(v ...interface{})) *FxEventLogger {
	return &FxEventLogger{logger: logger}
}

// Write writes the given bytes to the logger.
func (l *FxEventLogger) Write(p []byte) (n int, err error) {
	l.logger(string(p))
	return len(p), nil
}

type loggingParams struct {
	fx.In

	// Note to the reader: Don't use `optional:"true"` except if you truly understand how does it work
	// See https://github.com/uber-go/fx/issues/613. Logging is one of the few cases where it's acceptable.
	FxEventLogging *FxEventLogger `optional:"true"`
}

// FxLoggingOption returns an fx.Option that provides a logger that logs fx events.
// If FxEventLogger is provided, it will be used, otherwise nothing is logged.
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
