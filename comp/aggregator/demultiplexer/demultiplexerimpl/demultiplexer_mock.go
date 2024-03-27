// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package demultiplexerimpl

import (
	"go.uber.org/fx"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for this component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Provide(func(demux demultiplexerComp.Component) aggregator.Demultiplexer {
			return demux
		}),
	)
}

type mock struct {
	*aggregator.AgentDemultiplexer
	sender *sender.Sender
}

func (m *mock) SetDefaultSender(sender sender.Sender) {
	m.sender = &sender
}

func (m *mock) GetDefaultSender() (sender.Sender, error) {
	if m.sender != nil {
		return *m.sender, nil
	}
	return m.AgentDemultiplexer.GetDefaultSender()
}

func (m *mock) LazyGetSenderManager() (sender.SenderManager, error) {
	return m, nil
}

type mockDependencies struct {
	fx.In
	Log      log.Component
	Hostname hostname.Component
}

func newMock(deps mockDependencies) (demultiplexerComp.Component, demultiplexerComp.Mock, sender.SenderManager) {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true

	aggDeps := aggregator.TestDeps{
		Log:             deps.Log,
		Hostname:        deps.Hostname,
		SharedForwarder: defaultforwarder.NoopForwarder{},
		Compressor:      compressionimpl.NewMockCompressor(),
	}

	instance := &mock{AgentDemultiplexer: aggregator.InitAndStartAgentDemultiplexerForTest(aggDeps, opts, "")}
	return instance, instance, instance
}
