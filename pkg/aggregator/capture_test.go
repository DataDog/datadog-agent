// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCaptureDemultiplexerSharesManagerAndAdvertisesRunningMetrics(t *testing.T) {
	deps := createDemultiplexerAgentTestDeps(t)
	manager := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	defer manager.Close()
	opts := demuxTestOptions()
	opts.CaptureManager = manager
	opts.FlushInterval = time.Hour
	opts.NoAggregationPipelineWorkersCount = 2
	d := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	require.Empty(t, manager.Status().Capabilities)
	require.Same(t, manager, d.sharedSerializer.(*serializer.Serializer).LiveCapture)
	for _, s := range d.noAggSerializers {
		require.Same(t, manager, s.(*serializer.Serializer).LiveCapture)
	}
	go d.run()
	stopped := false
	defer func() {
		if !stopped {
			d.Stop()
		}
	}()
	require.Eventually(t, func() bool { return len(manager.Status().Capabilities) == 1 }, time.Second, 10*time.Millisecond)
	require.Equal(t, telemetrycapture.Capability{Stream: telemetrycapture.Metrics, Cadence: time.Hour}, manager.Status().Capabilities[0])
	c := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "demultiplexer-session"}
	_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: c, Streams: []telemetrycapture.Stream{telemetrycapture.Metrics}})
	require.NoError(t, err)
	_, err = manager.Activate(c)
	require.NoError(t, err)
	d.Stop()
	stopped = true
	require.Empty(t, manager.Status().Capabilities)
	require.False(t, manager.Enabled())
}

func TestCaptureDemultiplexerDisabledScheduleHasNoReadiness(t *testing.T) {
	deps := createDemultiplexerAgentTestDeps(t)
	manager := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	defer manager.Close()
	opts := demuxTestOptions()
	opts.CaptureManager = manager
	opts.FlushInterval = 0
	d := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	go d.run()
	d.Stop()
	require.Empty(t, manager.Status().Capabilities)
}
