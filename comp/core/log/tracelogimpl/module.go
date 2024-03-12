// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tracelogimpl provides a component that implements the log.Component for the trace-agent logger
package tracelogimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the log component in its Trace variant.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTraceLogger),
	)
}
