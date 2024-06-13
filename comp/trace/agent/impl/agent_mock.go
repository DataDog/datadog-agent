// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package agent

import (
	"context"
	"testing"

	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// NewMock creates a new mock agent component.
func NewMock(deps dependencies, t testing.TB) traceagent.Component { //nolint:revive // TODO fix revive unused-parameter
	telemetryCollector := telemetry.NewCollector(deps.Config.Object())

	// Several related non-components require a shared context to gracefully stop.
	ctx, cancel := context.WithCancel(context.Background())
	ag := &agent{
		Agent: pkgagent.NewAgent(
			ctx,
			deps.Config.Object(),
			telemetryCollector,
			&statsd.NoOpClient{},
		),
		cancel:             cancel,
		config:             deps.Config,
		statsd:             deps.Statsd,
		params:             deps.Params,
		shutdowner:         deps.Shutdowner,
		telemetryCollector: telemetryCollector,
	}

	// Temporary copy of pkg/trace/agent.NewTestAgent
	ag.TraceWriter.In = make(chan *writer.SampledChunks, 1000)
	ag.Concentrator.In = make(chan stats.Input, 1000)

	return component{}
}
