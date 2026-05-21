// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package demultiplexer

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Mock implements mock-specific methods.
type Mock interface {
	SetDefaultSender(sender.Sender)
	Stop(bool)
	Component
}

// FakeSamplerMock is an implementation of the Demultiplexer which is sending
// the time samples into a fake sampler, you can then use WaitForSamples() to retrieve
// the samples that the TimeSamplers should have received.
type FakeSamplerMock interface {
	aggregator.DemultiplexerWithAggregator

	WaitForSamples(timeout time.Duration) (ontime []metrics.MetricSample, timed []metrics.MetricSample)
	WaitForNumberOfSamples(ontimeCount, timedCount int, timeout time.Duration) (ontime []metrics.MetricSample, timed []metrics.MetricSample)
	Reset()

	GetAgentDemultiplexer() *aggregator.AgentDemultiplexer

	Stop(bool)
}
