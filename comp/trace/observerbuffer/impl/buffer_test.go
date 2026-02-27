// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerbufferimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	observerbuffer "github.com/DataDog/datadog-agent/comp/trace/observerbuffer/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// newTestBuffer creates a buffer for testing with recording enabled/disabled.
func newTestBuffer(t *testing.T, enabled bool) observerbuffer.Component {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"observer.recording.enabled": enabled,
		"observer.analysis.enabled":  enabled,
	})
	return NewComponent(Requires{Cfg: cfg, Log: logmock.New(t)}).Comp
}

func TestBufferAddAndDrainTraces(t *testing.T) {
	buf := newTestBuffer(t, true)

	// Add traces
	buf.AddTrace(&pb.TracerPayload{Env: "test1"})
	buf.AddTrace(&pb.TracerPayload{Env: "test2"})
	buf.AddTrace(&pb.TracerPayload{Env: "test3"})

	stats := buf.Stats()
	assert.Equal(t, 3, stats.TraceCount)
	assert.Equal(t, uint64(0), stats.TracesDropped)

	// Drain all
	traces, dropped, hasMore := buf.DrainTraces(0)
	assert.Len(t, traces, 3)
	assert.Equal(t, uint64(0), dropped)
	assert.False(t, hasMore)
	assert.Equal(t, "test1", traces[0].Payload.Env)
	assert.Equal(t, "test2", traces[1].Payload.Env)
	assert.Equal(t, "test3", traces[2].Payload.Env)

	// Buffer should be empty now
	stats = buf.Stats()
	assert.Equal(t, 0, stats.TraceCount)
}

func TestBufferDrainWithLimit(t *testing.T) {
	buf := newTestBuffer(t, true)

	// Add 5 traces
	for i := 0; i < 5; i++ {
		buf.AddTrace(&pb.TracerPayload{Env: "test"})
	}

	// Drain only 2
	traces, _, hasMore := buf.DrainTraces(2)
	assert.Len(t, traces, 2)
	assert.True(t, hasMore)

	// Drain remaining
	traces, _, hasMore = buf.DrainTraces(0)
	assert.Len(t, traces, 3)
	assert.False(t, hasMore)
}

func TestBufferProfiles(t *testing.T) {
	buf := newTestBuffer(t, true)

	// Add profiles
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1", ProfileType: "cpu"})
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p2", ProfileType: "heap"})

	stats := buf.Stats()
	assert.Equal(t, 2, stats.ProfileCount)

	// Drain and verify
	profiles, dropped, hasMore := buf.DrainProfiles(0)
	assert.Len(t, profiles, 2)
	assert.Equal(t, uint64(0), dropped)
	assert.False(t, hasMore)
	assert.Equal(t, "p1", profiles[0].ProfileID)
	assert.Equal(t, "p2", profiles[1].ProfileID)
}

func TestNoopBuffer(t *testing.T) {
	buf := newTestBuffer(t, false)

	// Operations should be no-ops
	buf.AddTrace(&pb.TracerPayload{Env: "test"})
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1"})

	traces, dropped, hasMore := buf.DrainTraces(0)
	assert.Nil(t, traces)
	assert.Equal(t, uint64(0), dropped)
	assert.False(t, hasMore)

	profiles, dropped, hasMore := buf.DrainProfiles(0)
	assert.Nil(t, profiles)
	assert.Equal(t, uint64(0), dropped)
	assert.False(t, hasMore)

	stats := buf.Stats()
	assert.Equal(t, 0, stats.TraceCount)
	assert.Equal(t, 0, stats.ProfileCount)
}

func TestBufferNilPayload(t *testing.T) {
	buf := newTestBuffer(t, true)

	// Adding nil should be safe
	buf.AddTrace(nil)

	stats := buf.Stats()
	assert.Equal(t, 0, stats.TraceCount)
}

func TestBufferReceivedAtTimestamp(t *testing.T) {
	buf := newTestBuffer(t, true)

	buf.AddTrace(&pb.TracerPayload{Env: "test"})
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1"})

	traces, _, _ := buf.DrainTraces(0)
	require.Len(t, traces, 1)
	assert.Greater(t, traces[0].ReceivedAtNs, int64(0))

	profiles, _, _ := buf.DrainProfiles(0)
	require.Len(t, profiles, 1)
	assert.Greater(t, profiles[0].ReceivedAtNs, int64(0))
}

func TestBufferNilConfig(t *testing.T) {
	// When Cfg is nil, should use defaults (which are disabled)
	provides := NewComponent(Requires{Cfg: nil})
	buf := provides.Comp

	// Should be noop buffer
	buf.AddTrace(&pb.TracerPayload{Env: "test"})
	stats := buf.Stats()
	assert.Equal(t, 0, stats.TraceCount)
}
