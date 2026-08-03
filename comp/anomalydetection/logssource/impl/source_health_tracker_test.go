// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"context"
	"testing"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type channelSourceHealth chan observer.LogSourceHealthObservation

func (c channelSourceHealth) ObserveLogSourceHealth(observation observer.LogSourceHealthObservation) {
	c <- observation
}

type captureSourceHealth struct {
	observations []observer.LogSourceHealthObservation
}

func (c *captureSourceHealth) ObserveLogSourceHealth(observation observer.LogSourceHealthObservation) {
	c.observations = append(c.observations, observation)
}

func healthTestSource(name, identifier string) *sources.LogSource {
	return sources.NewLogSource(name, &logsconfig.LogsConfig{
		Type:       logsconfig.DockerType,
		Identifier: identifier,
	})
}

func TestSourceHealthTrackerUsesTailerStatus(t *testing.T) {
	sink := &captureSourceHealth{}
	tracker := newSourceHealthTracker(sources.NewLogSources(), sink, 5*time.Second)
	source := healthTestSource("container", "container-123")
	tracker.add(source)

	tracker.sampleAt(10)
	require.Len(t, sink.observations, 1)
	assert.False(t, sink.observations[0].Healthy, "pending source status is not healthy evidence")

	source.Status.Success()
	tracker.sampleAt(15)
	require.Len(t, sink.observations, 2)
	assert.Equal(t, observer.LogSourceHealthObservation{SourceID: "container-123", Timestamp: 15, Healthy: true}, sink.observations[1])

	source.Status.Error(assert.AnError)
	tracker.sampleAt(20)
	require.Len(t, sink.observations, 3)
	assert.False(t, sink.observations[2].Healthy)
}

func TestSourceHealthTrackerHandlesOverlappingReplacement(t *testing.T) {
	sink := &captureSourceHealth{}
	tracker := newSourceHealthTracker(sources.NewLogSources(), sink, 5*time.Second)
	generic := healthTestSource("generic", "container-123")
	ad := healthTestSource("ad", "container-123")
	generic.Status.Success()
	ad.Status.Success()
	tracker.add(generic)
	tracker.add(ad)

	tracker.remove(generic)
	tracker.sampleIdentifier("container-123", 10)
	require.Len(t, sink.observations, 1)
	assert.True(t, sink.observations[0].Healthy, "remaining AD source keeps the identifier healthy")

	tracker.remove(ad)
	tracker.sampleIdentifier("container-123", 15)
	require.Len(t, sink.observations, 2)
	assert.False(t, sink.observations[1].Healthy)
}

func TestLogSourceIdentifierFallsBackToSourceName(t *testing.T) {
	source := healthTestSource("kubelet", "")
	assert.Equal(t, "kubelet", logSourceIdentifier(source))
}

func TestSourceHealthTrackerStartsAndStopsCleanly(t *testing.T) {
	logSources := sources.NewLogSources()
	observations := make(channelSourceHealth, 1)
	tracker := newSourceHealthTracker(logSources, observations, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	tracker.start(ctx)

	source := healthTestSource("container", "container-123")
	source.Status.Success()
	logSources.AddSource(source)
	got := <-observations
	assert.Equal(t, "container-123", got.SourceID)
	assert.True(t, got.Healthy)
	assert.NotZero(t, got.Timestamp)

	cancel()
	tracker.wait()
}
