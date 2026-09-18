// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// A sample sent right before WaitForPendingSamples must be processed by the
// time the call returns, and the call must be repeatable.
func TestWaitForPendingSamplesDrainsBeforeReturning(t *testing.T) {
	require := require.New(t)

	deps := createDemultiplexerAgentTestDeps(t)
	opts := demuxTestOptions()
	// InitAndStartAgentDemultiplexer (not initAgentDemultiplexer) is required
	// here: WaitForPendingSamples needs the worker's run() goroutine actually
	// running to service drainChan.
	demux := InitAndStartAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	defer demux.Stop()

	worker := demux.statsd.workers[0]
	require.Equal(0, worker.sampler.contextResolver.length())

	demux.AggregateSample(metrics.MetricSample{
		Name:      "test.metric",
		Value:     1,
		Mtype:     metrics.GaugeType,
		Timestamp: 1657099120.0,
		Tags:      []string{"tag:1"},
	})

	demux.WaitForPendingSamples()

	require.Len(worker.samplesChan, 0, "the sample must have been drained from samplesChan")
	require.Equal(1, worker.sampler.contextResolver.length(), "the sample must have been aggregated into a context, not just dequeued")

	// Calling it again with nothing pending must not block or panic.
	demux.WaitForPendingSamples()
}
