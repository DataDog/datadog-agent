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

	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
)

func newMock(deps dependencies, t testing.TB) Component { //nolint:revive // TODO fix revive unused-parameter
	telemetryCollector := telemetry.NewCollector(deps.Config.Object())

	// Several related non-components require a shared context to gracefully stop.
	ctx, cancel := context.WithCancel(context.Background())
	ag := &agent{
		Agent: pkgagent.NewAgent(
			ctx,
			deps.Config.Object(),
			telemetryCollector,
		),
		cancel:             cancel,
		config:             deps.Config,
		params:             deps.Params,
		shutdowner:         deps.Shutdowner,
		telemetryCollector: telemetryCollector,
	}

	// Temporary copy of pkg/trace/agent.NewTestAgent
	ag.TraceWriter.In = make(chan *writer.SampledChunks, 1000)
	ag.Concentrator.In = make(chan stats.Input, 1000)

	return component{}
}
