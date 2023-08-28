// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agent contains logs agent component.
package agent

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	// AddScheduler adds an AD scheduler to the logs agent
	AddScheduler(scheduler schedulers.Scheduler)

	// IsRunning returns true if the logs agent is running
	IsRunning() bool

	// GetMessageReceiver gets the diagnostic message receiver
	GetMessageReceiver() *diagnostic.BufferedMessageReceiver

	// Flush synchronously flushes the pipelines managed by the Logs Agent.
	Flush(ctx context.Context)

	// GetPipelineProvider gets the pipeline provider
	GetPipelineProvider() pipeline.Provider
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newLogsAgent),
)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)
