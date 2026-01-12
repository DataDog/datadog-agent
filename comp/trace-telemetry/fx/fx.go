// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the trace-telemetry component
package fx

import (
	"go.uber.org/fx"

	tracetelemetry "github.com/DataDog/datadog-agent/comp/trace-telemetry/def"
	tracetelemetryimpl "github.com/DataDog/datadog-agent/comp/trace-telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			tracetelemetryimpl.NewComponent,
		),
		fxutil.ProvideOptional[tracetelemetry.Component](),
		// force the instantiation of trace-telemetry
		fx.Invoke(func(_ tracetelemetry.Component) {}),
	)
}
