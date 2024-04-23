// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package logsagentpipeline

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry

// Component is the component type.
type Component interface {
	// GetPipelineProvider gets the pipeline provider
	GetPipelineProvider() pipeline.Provider
}

type LogsAgent interface {
	Component
	Start() error
	Stop()
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewLogsAgent))
}
