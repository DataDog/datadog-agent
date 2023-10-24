// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package demultiplexer

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"go.uber.org/fx"
)

type mock struct {
	Component
	sender *sender.Sender
}

func (m *mock) SetDefaultSender(sender sender.Sender) {
	m.sender = &sender
}

func (m *mock) GetDefaultSender() (sender.Sender, error) {
	if m.sender != nil {
		return *m.sender, nil
	}
	return m.Component.GetDefaultSender()
}

func (m *mock) LazyGetSenderManager() (sender.SenderManager, error) {
	return m, nil
}

type mockDependencies struct {
	fx.In
	AggregatorDeps aggregator.TestDeps
}

func newMock(deps mockDependencies) (Component, Mock) {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true

	demultiplexer := demultiplexer{
		AgentDemultiplexer: aggregator.InitAndStartAgentDemultiplexerForTest(deps.AggregatorDeps, opts, ""),
	}

	instance := &mock{Component: demultiplexer}
	return instance, instance
}
