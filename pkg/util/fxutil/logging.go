// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"os"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

// FxLoggingOption creates an fx.Option to configure the Fx logger, either to do nothing
// (the default) or to log to the console (when TRACE_FX is set).
func FxLoggingOption() fx.Option {
	return fx.WithLogger(
		func() fxevent.Logger {
			if os.Getenv("TRACE_FX") == "" {
				return fxevent.NopLogger
			}
			return &fxevent.ConsoleLogger{W: os.Stderr}
		},
	)
}
