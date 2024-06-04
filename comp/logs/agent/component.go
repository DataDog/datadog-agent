// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agent contains logs agent component.
package agent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	// AddScheduler adds an AD scheduler to the logs agent
	AddScheduler(scheduler schedulers.Scheduler)

	// Get the logs sources
	GetSources() *sources.LogSources

	// GetMessageReceiver gets the diagnostic message receiver
	GetMessageReceiver() *diagnostic.BufferedMessageReceiver

	// GetPipelineProvider gets the pipeline provider
	GetPipelineProvider() pipeline.Provider
}

// ServerlessLogsAgent is a compat version of the component for the serverless agent
type ServerlessLogsAgent interface {
	Component
	Start() error
	Stop()

	// Flush flushes synchronously the pipelines managed by the Logs Agent.
	Flush(ctx context.Context)
}
