// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerbuffer provides a ring buffer for trace and profile data
// that will be fetched by the core-agent's observer component.
package observerbuffer

// team: agent-apm

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// Component is the interface for the observer buffer that stores traces and profiles
// until they are fetched by the core-agent.
type Component interface {
	// AddTrace adds a trace payload to the buffer.
	// If the buffer is full, the oldest trace is dropped.
	AddTrace(payload *pb.TracerPayload)

	// AddProfile adds a profile to the buffer.
	// If the buffer is full, the oldest profile is dropped.
	AddProfile(profile ProfileData)

	// DrainTraces removes and returns up to maxItems traces from the buffer.
	// If maxItems is 0, all buffered traces are returned.
	// Returns the traces, count of dropped traces since last drain, and whether more data is available.
	DrainTraces(maxItems uint32) (traces []BufferedTrace, droppedCount uint64, hasMore bool)

	// DrainProfiles removes and returns up to maxItems profiles from the buffer.
	// If maxItems is 0, all buffered profiles are returned.
	// Returns the profiles, count of dropped profiles since last drain, and whether more data is available.
	DrainProfiles(maxItems uint32) (profiles []ProfileData, droppedCount uint64, hasMore bool)

	// Stats returns current buffer statistics.
	Stats() BufferStats
}

// BufferedTrace contains a trace payload with metadata.
type BufferedTrace struct {
	// Payload is the trace data from the tracer.
	Payload *pb.TracerPayload
	// ReceivedAtNs is when the trace was received (nanoseconds since epoch).
	ReceivedAtNs int64
}

// ProfileData contains profile metadata and optionally inline data.
// Profile format is language-agnostic (pprof for Go/Python, JFR for Java, etc.).
type ProfileData struct {
	// ProfileID is a unique identifier for this profile.
	ProfileID string
	// ProfileType identifies the kind of profile (cpu, heap, mutex, etc.).
	ProfileType string
	// Service is the name of the service that produced this profile.
	Service string
	// Env is the environment tag.
	Env string
	// Version is the application version.
	Version string
	// Hostname is where the profile was collected.
	Hostname string
	// ContainerID is the container where the profile was collected.
	ContainerID string
	// TimestampNs is when the profile was collected (nanoseconds since epoch).
	TimestampNs int64
	// DurationNs is the profile duration (nanoseconds).
	DurationNs int64
	// Tags are additional profile tags.
	Tags map[string]string
	// ContentType is the original Content-Type header (format is language-specific).
	ContentType string
	// InlineData contains the opaque binary profile data.
	InlineData []byte
	// ReceivedAtNs is when the trace-agent received this profile.
	ReceivedAtNs int64
}

// BufferStats contains statistics about the buffer.
type BufferStats struct {
	// TraceCount is the current number of buffered traces.
	TraceCount int
	// TraceCapacity is the maximum number of traces the buffer can hold.
	TraceCapacity int
	// TracesDropped is the total number of traces dropped due to overflow.
	TracesDropped uint64
	// ProfileCount is the current number of buffered profiles.
	ProfileCount int
	// ProfileCapacity is the maximum number of profiles the buffer can hold.
	ProfileCapacity int
	// ProfilesDropped is the total number of profiles dropped due to overflow.
	ProfilesDropped uint64
}
