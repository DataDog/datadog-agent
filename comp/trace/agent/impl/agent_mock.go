// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package agentimpl

import (
	"context"
	"testing"

	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	zstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type noopTraceWriter struct{}

func (n *noopTraceWriter) Run() {}

func (n *noopTraceWriter) Stop() {}

func (n *noopTraceWriter) WriteChunks(_ *writer.SampledChunks) {}

func (n *noopTraceWriter) FlushSync() error { return nil }

type noopConcentrator struct{}

func (c *noopConcentrator) Start()            {}
func (c *noopConcentrator) Stop()             {}
func (c *noopConcentrator) Add(_ stats.Input) {}

// NewMock creates a new mock agent component.
func NewMock(deps dependencies, _ testing.TB) traceagent.Component {
	telemetryCollector := telemetry.NewCollector(deps.Config.Object())

	// Several related non-components require a shared context to gracefully stop.
	ctx, cancel := context.WithCancel(context.Background())
	ag := component{
		Agent: pkgagent.NewAgent(
			ctx,
			deps.Config.Object(),
			telemetryCollector,
			&statsd.NoOpClient{},
			zstd.NewComponent(),
		),
		cancel:             cancel,
		config:             deps.Config,
		params:             deps.Params,
		telemetryCollector: telemetryCollector,
	}

	// Temporary copy of pkg/trace/agent.NewTestAgent
	ag.TraceWriter = &noopTraceWriter{}
	ag.Concentrator = &noopConcentrator{}

	return component{}
}
