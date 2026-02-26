// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerbufferimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	observerbuffer "github.com/DataDog/datadog-agent/comp/trace/observerbuffer/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBuffer creates a buffer for testing with the given configuration.
func newTestBuffer(t *testing.T, enabled bool, traceSize, profileSize int) observerbuffer.Component {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"apm_config.observer.enabled":             enabled,
		"apm_config.observer.trace_buffer_size":   traceSize,
		"apm_config.observer.profile_buffer_size": profileSize,
	})
	return NewComponent(Requires{Cfg: cfg, Log: logmock.New(t)}).Comp
}

func TestBufferAddAndDrainTraces(t *testing.T) {
	buf := newTestBuffer(t, true, 3, 3)

	// Add traces
	buf.AddTrace(&pb.TracerPayload{Env: "test1"})
	buf.AddTrace(&pb.TracerPayload{Env: "test2"})
	buf.AddTrace(&pb.TracerPayload{Env: "test3"})

	stats := buf.Stats()
	assert.Equal(t, 3, stats.TraceCount)
	assert.Equal(t, 3, stats.TraceCapacity)
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

func TestBufferOverflow(t *testing.T) {
	buf := newTestBuffer(t, true, 2, 2)

	// Add more traces than capacity
	buf.AddTrace(&pb.TracerPayload{Env: "test1"})
	buf.AddTrace(&pb.TracerPayload{Env: "test2"})
	buf.AddTrace(&pb.TracerPayload{Env: "test3"}) // Should drop test1

	stats := buf.Stats()
	assert.Equal(t, 2, stats.TraceCount)
	assert.Equal(t, uint64(1), stats.TracesDropped)

	// Drain and verify oldest was dropped
	traces, dropped, _ := buf.DrainTraces(0)
	assert.Len(t, traces, 2)
	assert.Equal(t, uint64(1), dropped)
	assert.Equal(t, "test2", traces[0].Payload.Env)
	assert.Equal(t, "test3", traces[1].Payload.Env)
}

func TestBufferDrainWithLimit(t *testing.T) {
	buf := newTestBuffer(t, true, 5, 5)

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
	buf := newTestBuffer(t, true, 3, 2)

	// Add profiles
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1", ProfileType: "cpu"})
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p2", ProfileType: "heap"})

	stats := buf.Stats()
	assert.Equal(t, 2, stats.ProfileCount)

	// Add one more to trigger overflow
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p3", ProfileType: "mutex"})

	stats = buf.Stats()
	assert.Equal(t, 2, stats.ProfileCount)
	assert.Equal(t, uint64(1), stats.ProfilesDropped)

	// Drain and verify
	profiles, dropped, _ := buf.DrainProfiles(0)
	assert.Len(t, profiles, 2)
	assert.Equal(t, uint64(1), dropped)
	assert.Equal(t, "p2", profiles[0].ProfileID)
	assert.Equal(t, "p3", profiles[1].ProfileID)
}

func TestNoopBuffer(t *testing.T) {
	// Disabled by default (enabled=false)
	buf := newTestBuffer(t, false, 3, 3)

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
	buf := newTestBuffer(t, true, 3, 3)

	// Adding nil should be safe
	buf.AddTrace(nil)

	stats := buf.Stats()
	assert.Equal(t, 0, stats.TraceCount)
}

func TestBufferReceivedAtTimestamp(t *testing.T) {
	buf := newTestBuffer(t, true, 3, 3)

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
