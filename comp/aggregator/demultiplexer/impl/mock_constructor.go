// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package demultiplexerimpl

import (
	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	defaultforwardernoop "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/noop-impl"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	metricscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// NewMock creates a mock demultiplexer without Fx dependency injection.
func NewMock(logComp log.Component, hostnameComp hostname.Component) demultiplexerComp.Mock {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true
	deps := aggregator.TestDeps{
		Log:                logComp,
		Hostname:           hostnameComp,
		SharedForwarder:    defaultforwardernoop.NewComponent(),
		MetricsCompression: metricscompressionmock.NewMockCompressor(),
		HaAgent:            haagentmock.NewMockHaAgent(),
		FilterList:         filterlistmock.NewMockFilterList(),
	}
	return &mock{AgentDemultiplexer: aggregator.InitAndStartAgentDemultiplexerForTest(deps, opts, "")}
}
