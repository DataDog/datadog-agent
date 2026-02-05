// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerbufferimpl

import (
	"testing"

	observerbuffer "github.com/DataDog/datadog-agent/comp/trace/observerbuffer/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBuffer creates a buffer for testing with the given configuration.
func newTestBuffer(t *testing.T, enabled bool, traceSize, profileSize int) observerbuffer.Component {
	return &bufferImpl{
		traceBuffer:   make(map[string]observerbuffer.BufferedTrace, traceSize),
		profileBuffer: make(map[string]observerbuffer.ProfileData, profileSize),
		traceCap:      traceSize,
		profileCap:    profileSize,
	}
}

func TestBufferAddAndDrainTraces(t *testing.T) {
	buf := newTestBuffer(t, true, 3, 3)

	// Add traces with different services
	buf.AddTrace(&pb.TracerPayload{
		Env:    "test1",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})
	buf.AddTrace(&pb.TracerPayload{
		Env:    "test2",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service2"}}}},
	})
	buf.AddTrace(&pb.TracerPayload{
		Env:    "test3",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service3"}}}},
	})

	stats := buf.Stats()
	assert.Equal(t, 3, stats.TraceCount)
	assert.Equal(t, 3, stats.TraceCapacity)
	assert.Equal(t, uint64(0), stats.TracesDropped)

	// Drain all
	traces, dropped, hasMore := buf.DrainTraces(0)
	assert.Len(t, traces, 3)
	assert.Equal(t, uint64(0), dropped)
	assert.False(t, hasMore)

	// Verify all envs are present (order may vary due to map iteration)
	envs := make(map[string]bool)
	for _, trace := range traces {
		envs[trace.Payload.Env] = true
	}
	assert.True(t, envs["test1"])
	assert.True(t, envs["test2"])
	assert.True(t, envs["test3"])

	// Buffer should be empty now
	stats = buf.Stats()
	assert.Equal(t, 0, stats.TraceCount)
}

func TestBufferOverflow(t *testing.T) {
	buf := newTestBuffer(t, true, 2, 2)

	// Add more services than capacity
	buf.AddTrace(&pb.TracerPayload{
		Env:    "test1",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})
	buf.AddTrace(&pb.TracerPayload{
		Env:    "test2",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service2"}}}},
	})
	// This should be dropped since buffer is at capacity and it's a new service
	buf.AddTrace(&pb.TracerPayload{
		Env:    "test3",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service3"}}}},
	})

	stats := buf.Stats()
	assert.Equal(t, 2, stats.TraceCount) // Only service1 and service2
	assert.Equal(t, uint64(1), stats.TracesDropped)

	// Drain and verify service3 was dropped
	traces, dropped, _ := buf.DrainTraces(0)
	assert.Len(t, traces, 2)
	assert.Equal(t, uint64(1), dropped)

	// Verify we have service1 and service2, not service3
	envs := make(map[string]bool)
	for _, trace := range traces {
		envs[trace.Payload.Env] = true
	}
	assert.True(t, envs["test1"])
	assert.True(t, envs["test2"])
	assert.False(t, envs["test3"])
}

func TestBufferDrainWithLimit(t *testing.T) {
	buf := newTestBuffer(t, true, 5, 5)

	// Add 5 traces from different services
	for i := 0; i < 5; i++ {
		buf.AddTrace(&pb.TracerPayload{
			Env:    "test",
			Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service" + string(rune('1'+i))}}}},
		})
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

	// Add profiles with different service:type combinations
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1", Service: "service1", ProfileType: "cpu"})
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p2", Service: "service1", ProfileType: "heap"})

	stats := buf.Stats()
	assert.Equal(t, 2, stats.ProfileCount)

	// Add one more service:type to trigger overflow (new combination)
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p3", Service: "service2", ProfileType: "cpu"})

	stats = buf.Stats()
	assert.Equal(t, 2, stats.ProfileCount) // Only 2 profiles kept
	assert.Equal(t, uint64(1), stats.ProfilesDropped)

	// Drain and verify we have the first two (p3 was dropped)
	profiles, dropped, _ := buf.DrainProfiles(0)
	assert.Len(t, profiles, 2)
	assert.Equal(t, uint64(1), dropped)

	// Verify we have service1's profiles (order may vary due to map iteration)
	profileIDs := make(map[string]bool)
	for _, profile := range profiles {
		profileIDs[profile.ProfileID] = true
	}
	assert.True(t, profileIDs["p1"])
	assert.True(t, profileIDs["p2"])
	assert.False(t, profileIDs["p3"]) // p3 was dropped
}

func TestBufferCapacityZero(t *testing.T) {
	// Test buffer with zero capacity
	buf := newTestBuffer(t, true, 0, 0)

	// Add traces - should be dropped since capacity is 0
	buf.AddTrace(&pb.TracerPayload{
		Env:    "test",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})

	stats := buf.Stats()
	assert.Equal(t, 0, stats.TraceCount)
	assert.Equal(t, uint64(1), stats.TracesDropped) // Should be dropped

	// Add profile - should be dropped since capacity is 0
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1"})

	stats = buf.Stats()
	assert.Equal(t, 0, stats.ProfileCount)
	assert.Equal(t, uint64(1), stats.ProfilesDropped)
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

	buf.AddTrace(&pb.TracerPayload{
		Env:    "test",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1"})

	traces, _, _ := buf.DrainTraces(0)
	require.Len(t, traces, 1)
	assert.Greater(t, traces[0].ReceivedAtNs, int64(0))

	profiles, _, _ := buf.DrainProfiles(0)
	require.Len(t, profiles, 1)
	assert.Greater(t, profiles[0].ReceivedAtNs, int64(0))
}

func TestBufferLastTracePerService(t *testing.T) {
	buf := newTestBuffer(t, true, 5, 5)

	// Add multiple traces for the same service
	buf.AddTrace(&pb.TracerPayload{
		Env:    "env1",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})
	buf.AddTrace(&pb.TracerPayload{
		Env:    "env2",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})
	buf.AddTrace(&pb.TracerPayload{
		Env:    "env3",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})

	stats := buf.Stats()
	assert.Equal(t, 1, stats.TraceCount)            // Only one trace for service1
	assert.Equal(t, uint64(0), stats.TracesDropped) // No traces dropped (just replaced)

	// Drain and verify we have the last trace
	traces, _, _ := buf.DrainTraces(0)
	assert.Len(t, traces, 1)
	assert.Equal(t, "env3", traces[0].Payload.Env) // Should be the last one added
}

func TestBufferMultipleServicesReplacement(t *testing.T) {
	buf := newTestBuffer(t, true, 3, 3)

	// Add traces for different services
	buf.AddTrace(&pb.TracerPayload{
		Env:    "env1",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})
	buf.AddTrace(&pb.TracerPayload{
		Env:    "env2",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service2"}}}},
	})

	// Replace service1's trace
	buf.AddTrace(&pb.TracerPayload{
		Env:    "env1-updated",
		Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{Service: "service1"}}}},
	})

	stats := buf.Stats()
	assert.Equal(t, 2, stats.TraceCount) // Still 2 services
	assert.Equal(t, uint64(0), stats.TracesDropped)

	// Drain and verify
	traces, _, _ := buf.DrainTraces(0)
	assert.Len(t, traces, 2)

	// Find and verify service1's trace was updated
	envs := make(map[string]bool)
	for _, trace := range traces {
		envs[trace.Payload.Env] = true
	}
	assert.True(t, envs["env1-updated"])
	assert.True(t, envs["env2"])
	assert.False(t, envs["env1"]) // Old service1 trace should be replaced
}

func TestBufferEmptyService(t *testing.T) {
	buf := newTestBuffer(t, true, 3, 3)

	// Add trace without service (should use "unknown")
	buf.AddTrace(&pb.TracerPayload{Env: "test"})

	stats := buf.Stats()
	assert.Equal(t, 1, stats.TraceCount)

	// Add another trace without service (should replace the first one)
	buf.AddTrace(&pb.TracerPayload{Env: "test2"})

	stats = buf.Stats()
	assert.Equal(t, 1, stats.TraceCount)

	// Drain and verify we have the last one
	traces, _, _ := buf.DrainTraces(0)
	assert.Len(t, traces, 1)
	assert.Equal(t, "test2", traces[0].Payload.Env)
}

func TestBufferLastProfilePerServiceType(t *testing.T) {
	buf := newTestBuffer(t, true, 5, 5)

	// Add multiple profiles for the same service:type
	buf.AddProfile(observerbuffer.ProfileData{
		ProfileID:   "p1",
		Service:     "service1",
		ProfileType: "cpu",
		Env:         "env1",
	})
	buf.AddProfile(observerbuffer.ProfileData{
		ProfileID:   "p2",
		Service:     "service1",
		ProfileType: "cpu",
		Env:         "env2",
	})
	buf.AddProfile(observerbuffer.ProfileData{
		ProfileID:   "p3",
		Service:     "service1",
		ProfileType: "cpu",
		Env:         "env3",
	})

	stats := buf.Stats()
	assert.Equal(t, 1, stats.ProfileCount)            // Only one profile for service1:cpu
	assert.Equal(t, uint64(0), stats.ProfilesDropped) // No profiles dropped (just replaced)

	// Drain and verify we have the last profile
	profiles, _, _ := buf.DrainProfiles(0)
	assert.Len(t, profiles, 1)
	assert.Equal(t, "p3", profiles[0].ProfileID) // Should be the last one added
	assert.Equal(t, "env3", profiles[0].Env)
}

func TestBufferMultipleProfileTypes(t *testing.T) {
	buf := newTestBuffer(t, true, 5, 5)

	// Add profiles for the same service but different types
	buf.AddProfile(observerbuffer.ProfileData{
		ProfileID:   "p1",
		Service:     "service1",
		ProfileType: "cpu",
	})
	buf.AddProfile(observerbuffer.ProfileData{
		ProfileID:   "p2",
		Service:     "service1",
		ProfileType: "heap",
	})
	buf.AddProfile(observerbuffer.ProfileData{
		ProfileID:   "p3",
		Service:     "service1",
		ProfileType: "mutex",
	})

	stats := buf.Stats()
	assert.Equal(t, 3, stats.ProfileCount) // 3 different service:type combinations
	assert.Equal(t, uint64(0), stats.ProfilesDropped)

	// Replace cpu profile
	buf.AddProfile(observerbuffer.ProfileData{
		ProfileID:   "p1-updated",
		Service:     "service1",
		ProfileType: "cpu",
	})

	stats = buf.Stats()
	assert.Equal(t, 3, stats.ProfileCount) // Still 3 combinations
	assert.Equal(t, uint64(0), stats.ProfilesDropped)

	// Drain and verify
	profiles, _, _ := buf.DrainProfiles(0)
	assert.Len(t, profiles, 3)

	// Verify all profile types are present and cpu was updated
	profileIDs := make(map[string]bool)
	for _, profile := range profiles {
		profileIDs[profile.ProfileID] = true
	}
	assert.True(t, profileIDs["p1-updated"]) // CPU profile was replaced
	assert.True(t, profileIDs["p2"])
	assert.True(t, profileIDs["p3"])
	assert.False(t, profileIDs["p1"]) // Old CPU profile should be replaced
}

func TestBufferProfileEmptyServiceType(t *testing.T) {
	buf := newTestBuffer(t, true, 3, 3)

	// Add profile without service or type (should use "unknown:unknown")
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p1"})

	stats := buf.Stats()
	assert.Equal(t, 1, stats.ProfileCount)

	// Add another profile without service or type (should replace the first one)
	buf.AddProfile(observerbuffer.ProfileData{ProfileID: "p2"})

	stats = buf.Stats()
	assert.Equal(t, 1, stats.ProfileCount)

	// Drain and verify we have the last one
	profiles, _, _ := buf.DrainProfiles(0)
	assert.Len(t, profiles, 1)
	assert.Equal(t, "p2", profiles[0].ProfileID)
}
