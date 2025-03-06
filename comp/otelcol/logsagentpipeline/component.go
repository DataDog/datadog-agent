// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package logsagentpipeline contains logs agent pipeline component
package logsagentpipeline

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// team: opentelemetry-agent

// Component is the component type.
type Component interface {
	// GetPipelineProvider gets the pipeline provider
	GetPipelineProvider() pipeline.Provider
}

// LogsAgent is a compat version of component for non fx usage
type LogsAgent interface {
	Component

	// Start sets up the logs agent and starts its pipelines
	Start(context.Context) error

	// Stop stops the logs agent and all elements of the data pipeline
	Stop(context.Context) error
}
