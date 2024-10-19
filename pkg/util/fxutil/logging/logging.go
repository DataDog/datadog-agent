// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logging provides a logger that logs fx events.
package logging

import (
	"os"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

// DefaultFxLoggingOption creates an fx.Option to configure the Fx logger, either to do nothing
// (the default) or to log to the console (when TRACE_FX is set at `1`).
func DefaultFxLoggingOption() fx.Option {
	return fx.Options(
		fx.WithLogger(
			func() fxevent.Logger {
				if os.Getenv("TRACE_FX") == "1" {
					return &fxevent.ConsoleLogger{W: os.Stderr}
				}
				return fxevent.NopLogger
			},
		),
	)
}

// Logger is a logger that logs fx events.
type Logger interface {
	Debug(v ...interface{})
}

// EnableFxLoggingOnDebug enables the logs for FX events when log_level is debug.
// This function requires that DefaultFxLoggingOption is part of the fx Options.
// This function uses generics to avoid depending on the logger component.
// When TRACE_FX is set to 0, it will disable the logging.
func EnableFxLoggingOnDebug[T Logger]() fx.Option {
	return fx.Decorate(func(logger T) fxevent.Logger {
		if os.Getenv("TRACE_FX") == "0" {
			return fxevent.NopLogger
		}
		return &fxevent.ConsoleLogger{W: fxEventLogger{logger: logger}}
	})
}

type fxEventLogger struct {
	logger Logger
}

func (l fxEventLogger) Write(p []byte) (n int, err error) {
	l.logger.Debug(string(p))
	return len(p), nil
}
