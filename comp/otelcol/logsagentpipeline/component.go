// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package logsagentpipeline contains logs agent pipeline component
package logsagentpipeline

import (
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// team: opentelemetry

// Component is the component type.
type Component interface {
	// GetPipelineProvider gets the pipeline provider
	GetPipelineProvider() pipeline.Provider
}
