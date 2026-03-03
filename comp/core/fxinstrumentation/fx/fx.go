// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the fxinstrumentation component
package fx

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

// Module enables the Fx initialization spans to be sent to Datadog.
// The tracer is implemented in pkg/util/fxutil/logging/tracer.go.
// Traces are sent only when the environment variable DD_FX_TRACING_ENABLED is set to true and this module added to the Fx application.
func Module() fxutil.Module {
	return fxutil.Component(
		// No need to call ProvideComponentConstructor because this module is not a component.
		fx.Invoke(func(logger fxevent.Logger, config config.Component) {
			if instrumentedLogger, ok := logger.(*logging.FxTracingLogger); ok {
				instrumentedLogger.EnableSpansSending(config.GetString("apm_config.receiver_port"))
			}
		}),
	)
}
