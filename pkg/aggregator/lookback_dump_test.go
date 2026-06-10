// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// newDumpTestDemux builds a minimal demultiplexer wired to a capturing
// serializer and an enabled lookback buffer + shadow sender manager, without
// spinning up the full aggregator/forwarder machinery.
func newDumpTestDemux(t *testing.T) (*AgentDemultiplexer, *MockSerializerIterableSerie) {
	t.Helper()
	configmock.New(t)
	s := &MockSerializerIterableSerie{}
	buffer := ringbuffer.New(ringbuffer.Options{})
	demux := &AgentDemultiplexer{
		log:                   logmock.New(t),
		lookbackBuffer:        buffer,
		lookbackSenderManager: lookbacksender.NewSenderManager(context.Background(), "h", buffer, nil),
		dataOutputs:           dataOutputs{sharedSerializer: s},
	}
	return demux, s
}

// retainViaShadowSender pushes a sample into the lookback buffer through the
// shadow sender path (the only path that feeds it).
func retainViaShadowSender(t *testing.T, demux *AgentDemultiplexer, id checkid.ID, name string, value float64) {
	t.Helper()
	sender, err := demux.LookbackSenderManager().GetSender(id)
	require.NoError(t, err)
	sender.Gauge(name, value, "", nil)
	sender.Commit()
}

func TestDumpLookbackSendsRetainedSamples(t *testing.T) {
	demux, s := newDumpTestDemux(t)

	retainViaShadowSender(t, demux, "dump-check", "dump.gauge", 1)
	retainViaShadowSender(t, demux, "dump-check", "dump.other", 2)

	count, err := demux.DumpLookback()
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	require.Len(t, s.series, 2, "both retained samples should be sent through the serializer")

	byName := map[string]float64{}
	for _, serie := range s.series {
		require.Len(t, serie.Points, 1)
		byName[serie.Name] = serie.Points[0].Value
	}
	assert.Equal(t, 1.0, byName["dump.gauge"])
	assert.Equal(t, 2.0, byName["dump.other"])

	// Dump is non-destructive: a second dump resends the same samples.
	count2, err := demux.DumpLookback()
	require.NoError(t, err)
	assert.Equal(t, 2, count2)
}

func TestDumpLookbackEmptyBufferSendsNothing(t *testing.T) {
	demux, s := newDumpTestDemux(t)

	count, err := demux.DumpLookback()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, s.series)
}

func TestDumpLookbackDisabledReturnsError(t *testing.T) {
	configmock.New(t)
	demux := &AgentDemultiplexer{
		log:         logmock.New(t),
		dataOutputs: dataOutputs{sharedSerializer: &MockSerializerIterableSerie{}},
	}
	_, err := demux.DumpLookback()
	require.Error(t, err)
}
