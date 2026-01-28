// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerbufferimpl implements the observer buffer component.
package observerbufferimpl

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	observerbuffer "github.com/DataDog/datadog-agent/comp/trace/observerbuffer/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// Config holds configuration for the observer buffer.
type Config struct {
	// TraceBufferSize is the maximum number of trace payloads to buffer.
	TraceBufferSize int
	// ProfileBufferSize is the maximum number of profiles to buffer.
	ProfileBufferSize int
	// Enabled controls whether buffering is active.
	Enabled bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		TraceBufferSize:   1000,
		ProfileBufferSize: 100,
		Enabled:           false, // Disabled by default until observer integration is ready
	}
}

// Requires defines the dependencies for the observer buffer component.
type Requires struct {
	// Cfg is the agent config component.
	Cfg config.Component
}

// Provides defines the output of the observer buffer component.
type Provides struct {
	Comp observerbuffer.Component
}

// NewComponent creates a new observer buffer component.
func NewComponent(reqs Requires) Provides {
	cfg := DefaultConfig()

	// Read configuration from apm_config.observer.*
	if reqs.Cfg != nil {
		cfg.Enabled = reqs.Cfg.GetBool("apm_config.observer.enabled")
		if traceSize := reqs.Cfg.GetInt("apm_config.observer.trace_buffer_size"); traceSize > 0 {
			cfg.TraceBufferSize = traceSize
		}
		if profileSize := reqs.Cfg.GetInt("apm_config.observer.profile_buffer_size"); profileSize > 0 {
			cfg.ProfileBufferSize = profileSize
		}
	}

	if !cfg.Enabled {
		return Provides{Comp: &noopBuffer{}}
	}

	return Provides{
		Comp: &bufferImpl{
			traceBuffer:   make([]observerbuffer.BufferedTrace, 0, cfg.TraceBufferSize),
			profileBuffer: make([]observerbuffer.ProfileData, 0, cfg.ProfileBufferSize),
			traceCap:      cfg.TraceBufferSize,
			profileCap:    cfg.ProfileBufferSize,
		},
	}
}

// bufferImpl is the ring buffer implementation.
type bufferImpl struct {
	mu sync.Mutex

	traceBuffer   []observerbuffer.BufferedTrace
	profileBuffer []observerbuffer.ProfileData

	traceCap   int
	profileCap int

	tracesDropped   atomic.Uint64
	profilesDropped atomic.Uint64

	// Counters for dropped items since last drain (reset on drain)
	traceDroppedSinceDrain   uint64
	profileDroppedSinceDrain uint64
}

// AddTrace adds a trace payload to the buffer.
func (b *bufferImpl) AddTrace(payload *pb.TracerPayload) {
	if payload == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// If buffer is full, drop the oldest entry
	if len(b.traceBuffer) >= b.traceCap {
		// Shift buffer left by one (drop oldest)
		copy(b.traceBuffer, b.traceBuffer[1:])
		b.traceBuffer = b.traceBuffer[:len(b.traceBuffer)-1]
		b.tracesDropped.Add(1)
		b.traceDroppedSinceDrain++
	}

	b.traceBuffer = append(b.traceBuffer, observerbuffer.BufferedTrace{
		Payload:      payload,
		ReceivedAtNs: time.Now().UnixNano(),
	})
}

// AddProfile adds a profile to the buffer.
func (b *bufferImpl) AddProfile(profile observerbuffer.ProfileData) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If buffer is full, drop the oldest entry
	if len(b.profileBuffer) >= b.profileCap {
		// Shift buffer left by one (drop oldest)
		copy(b.profileBuffer, b.profileBuffer[1:])
		b.profileBuffer = b.profileBuffer[:len(b.profileBuffer)-1]
		b.profilesDropped.Add(1)
		b.profileDroppedSinceDrain++
	}

	profile.ReceivedAtNs = time.Now().UnixNano()
	b.profileBuffer = append(b.profileBuffer, profile)
}

// DrainTraces removes and returns up to maxItems traces from the buffer.
func (b *bufferImpl) DrainTraces(maxItems uint32) (traces []observerbuffer.BufferedTrace, droppedCount uint64, hasMore bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	droppedCount = b.traceDroppedSinceDrain
	b.traceDroppedSinceDrain = 0

	if len(b.traceBuffer) == 0 {
		return nil, droppedCount, false
	}

	count := len(b.traceBuffer)
	if maxItems > 0 && int(maxItems) < count {
		count = int(maxItems)
		hasMore = true
	}

	// Copy the traces to return
	traces = make([]observerbuffer.BufferedTrace, count)
	copy(traces, b.traceBuffer[:count])

	// Remove drained traces from buffer
	remaining := len(b.traceBuffer) - count
	if remaining > 0 {
		copy(b.traceBuffer, b.traceBuffer[count:])
		hasMore = true
	}
	b.traceBuffer = b.traceBuffer[:remaining]

	return traces, droppedCount, hasMore
}

// DrainProfiles removes and returns up to maxItems profiles from the buffer.
func (b *bufferImpl) DrainProfiles(maxItems uint32) (profiles []observerbuffer.ProfileData, droppedCount uint64, hasMore bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	droppedCount = b.profileDroppedSinceDrain
	b.profileDroppedSinceDrain = 0

	if len(b.profileBuffer) == 0 {
		return nil, droppedCount, false
	}

	count := len(b.profileBuffer)
	if maxItems > 0 && int(maxItems) < count {
		count = int(maxItems)
		hasMore = true
	}

	// Copy the profiles to return
	profiles = make([]observerbuffer.ProfileData, count)
	copy(profiles, b.profileBuffer[:count])

	// Remove drained profiles from buffer
	remaining := len(b.profileBuffer) - count
	if remaining > 0 {
		copy(b.profileBuffer, b.profileBuffer[count:])
		hasMore = true
	}
	b.profileBuffer = b.profileBuffer[:remaining]

	return profiles, droppedCount, hasMore
}

// Stats returns current buffer statistics.
func (b *bufferImpl) Stats() observerbuffer.BufferStats {
	b.mu.Lock()
	defer b.mu.Unlock()

	return observerbuffer.BufferStats{
		TraceCount:      len(b.traceBuffer),
		TraceCapacity:   b.traceCap,
		TracesDropped:   b.tracesDropped.Load(),
		ProfileCount:    len(b.profileBuffer),
		ProfileCapacity: b.profileCap,
		ProfilesDropped: b.profilesDropped.Load(),
	}
}

// noopBuffer is a no-op implementation when buffering is disabled.
type noopBuffer struct{}

func (n *noopBuffer) AddTrace(_ *pb.TracerPayload)        {}
func (n *noopBuffer) AddProfile(_ observerbuffer.ProfileData) {}

func (n *noopBuffer) DrainTraces(_ uint32) ([]observerbuffer.BufferedTrace, uint64, bool) {
	return nil, 0, false
}

func (n *noopBuffer) DrainProfiles(_ uint32) ([]observerbuffer.ProfileData, uint64, bool) {
	return nil, 0, false
}

func (n *noopBuffer) Stats() observerbuffer.BufferStats {
	return observerbuffer.BufferStats{}
}
