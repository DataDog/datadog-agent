// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package demultiplexerimpl

import (
	"context"
	"time"

	"go.uber.org/fx"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// FakeSamplerMockModule defines the fx options for FakeSamplerMock.
func FakeSamplerMockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newFakeSamplerMock),
		fx.Provide(func(demux demultiplexerComp.FakeSamplerMock) aggregator.Demultiplexer {
			return demux
		}),
	)
}

type fakeSamplerMockDependencies struct {
	fx.In
	Lc         fx.Lifecycle
	Log        log.Component
	Hostname   hostname.Component
	Compressor compression.Component
}

type fakeSamplerMock struct {
	*TestAgentDemultiplexer
	stopped bool
}

func (f *fakeSamplerMock) GetAgentDemultiplexer() *aggregator.AgentDemultiplexer {
	return f.TestAgentDemultiplexer.AgentDemultiplexer
}

func (f *fakeSamplerMock) Stop(flush bool) {
	if !f.stopped {
		f.TestAgentDemultiplexer.Stop(flush)
		f.stopped = true
	}
}

func newFakeSamplerMock(deps fakeSamplerMockDependencies) demultiplexerComp.FakeSamplerMock {
	demux := initTestAgentDemultiplexerWithFlushInterval(deps.Log, deps.Hostname, deps.Compressor, time.Hour)
	mock := &fakeSamplerMock{
		TestAgentDemultiplexer: demux,
	}

	deps.Lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			mock.Stop(false)
			return nil
		}})
	return mock
}
