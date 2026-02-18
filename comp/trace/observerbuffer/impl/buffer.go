// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerbufferimpl implements the observer buffer component.
package observerbufferimpl

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	observerbuffer "github.com/DataDog/datadog-agent/comp/trace/observerbuffer/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config holds configuration for the observer buffer.
type Config struct {
	// TraceBufferSize is the maximum number of trace payloads to buffer.
	TraceBufferSize int
	// ProfileBufferSize is the maximum number of profiles to buffer.
	ProfileBufferSize int
	// StatsBufferSize is the maximum number of stats payloads to buffer.
	StatsBufferSize int
	// Enabled controls whether buffering is active.
	Enabled bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		TraceBufferSize:   1000,
		ProfileBufferSize: 100,
		StatsBufferSize:   100,
		Enabled:           false, // Disabled by default until observer integration is ready
	}
}

// Requires defines the dependencies for the observer buffer component.
type Requires struct {
	// Cfg is the agent config component.
	Cfg config.Component
	Log logdef.Component
}

// Provides defines the output of the observer buffer component.
type Provides struct {
	Comp observerbuffer.Component
}

// NewComponent creates a new observer buffer component.
func NewComponent(reqs Requires) Provides {
	cfg := DefaultConfig()

	// Read configuration from apm_config.observer.*
	cfg.Enabled = reqs.Cfg.GetBool("apm_config.observer.enabled")
	if traceSize := reqs.Cfg.GetInt("apm_config.observer.trace_buffer_size"); traceSize > 0 {
		cfg.TraceBufferSize = traceSize
	}
	if profileSize := reqs.Cfg.GetInt("apm_config.observer.profile_buffer_size"); profileSize > 0 {
		cfg.ProfileBufferSize = profileSize
	}
	if statsSize := reqs.Cfg.GetInt("apm_config.observer.stats_buffer_size"); statsSize > 0 {
		cfg.StatsBufferSize = statsSize
	}

	reqs.Log.Infof("Observer buffer configured: enabled=%v, trace_buffer_size=%d, profile_buffer_size=%d, stats_buffer_size=%d", cfg.Enabled, cfg.TraceBufferSize, cfg.ProfileBufferSize, cfg.StatsBufferSize)

	if !cfg.Enabled {
		return Provides{Comp: &noopBuffer{}}
	}

	return Provides{
		Comp: &bufferImpl{
			traceBuffer:   make([]observerbuffer.BufferedTrace, 0, cfg.TraceBufferSize),
			profileBuffer: make([]observerbuffer.ProfileData, 0, cfg.ProfileBufferSize),
			statsBuffer:   make([]observerbuffer.BufferedStats, 0, cfg.StatsBufferSize),
			traceCap:      cfg.TraceBufferSize,
			profileCap:    cfg.ProfileBufferSize,
			statsCap:      cfg.StatsBufferSize,
		},
	}
}

// bufferImpl is the ring buffer implementation.
type bufferImpl struct {
	mu sync.Mutex

	traceBuffer   []observerbuffer.BufferedTrace
	profileBuffer []observerbuffer.ProfileData
	statsBuffer   []observerbuffer.BufferedStats

	traceCap   int
	profileCap int
	statsCap   int

	tracesDropped   atomic.Uint64
	profilesDropped atomic.Uint64
	statsDropped    atomic.Uint64

	// Counters for dropped items since last drain (reset on drain)
	traceDroppedSinceDrain   uint64
	profileDroppedSinceDrain uint64
	statsDroppedSinceDrain   uint64
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

// AddRawProfile adds a raw profile from an HTTP request to the buffer.
// This method extracts metadata from headers and stores the raw body.
// Note: Parsing multipart form data is deferred to the core-agent to avoid complexity here.
func (b *bufferImpl) AddRawProfile(body []byte, headers map[string][]string) {
	if len(body) == 0 {
		return
	}

	profile := observerbuffer.ProfileData{
		ContentType:  getFirstHeader(headers, "Content-Type"),
		InlineData:   make([]byte, len(body)),
		ReceivedAtNs: time.Now().UnixNano(),
		Tags:         make(map[string]string),
	}
	copy(profile.InlineData, body)

	// Try to parse multipart form data to extract metadata
	contentType := getFirstHeader(headers, "Content-Type")
	log.Debugf("[observerbuffer] Profile Content-Type: %s", contentType)
	if mediaType, params, err := mime.ParseMediaType(contentType); err == nil && mediaType == "multipart/form-data" {
		boundary := params["boundary"]
		log.Debugf("[observerbuffer] Parsed boundary: %s", boundary)
		if boundary != "" {
			if metadata := parseProfileMetadata(body, boundary); metadata != nil {
				log.Debugf("[observerbuffer] Extracted metadata - Service: %s, Type: %s, Env: %s",
					metadata.Service, metadata.ProfileType, metadata.Env)
				profile.ProfileID = metadata.ProfileID
				profile.ProfileType = metadata.ProfileType
				profile.Service = metadata.Service
				profile.Env = metadata.Env
				profile.Version = metadata.Version
				profile.Hostname = metadata.Hostname
				profile.DurationNs = metadata.DurationNs
				profile.TimestampNs = metadata.TimestampNs
				// Merge metadata tags with profile tags
				for k, v := range metadata.Tags {
					profile.Tags[k] = v
				}
			} else {
				log.Debugf("[observerbuffer] parseProfileMetadata returned nil")
			}
		}
	} else if err != nil {
		log.Debugf("[observerbuffer] Failed to parse Content-Type: %v", err)
	}

	// Extract container tags from headers
	if containerTags := getFirstHeader(headers, "X-Datadog-Container-Tags"); containerTags != "" {
		profile.Tags["_container_tags"] = containerTags
	}

	// Extract additional tags from headers
	if additionalTags := getFirstHeader(headers, "X-Datadog-Additional-Tags"); additionalTags != "" {
		profile.Tags["_additional_tags"] = additionalTags
	}

	b.AddProfile(profile)
}

// getFirstHeader returns the first value for a header key, or empty string if not found.
func getFirstHeader(headers map[string][]string, key string) string {
	if values, ok := headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

// profileMetadata holds extracted profile metadata from the multipart form event field.
type profileMetadata struct {
	ProfileID   string
	ProfileType string
	Service     string
	Env         string
	Version     string
	Hostname    string
	TimestampNs int64 // Profile start time (nanoseconds since epoch)
	DurationNs  int64
	Tags        map[string]string
}

// parseProfileMetadata extracts profile metadata from multipart form data.
// The Datadog profiling intake expects an "event" field with JSON metadata.
func parseProfileMetadata(body []byte, boundary string) *profileMetadata {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	log.Debugf("[observerbuffer] Starting multipart parse with boundary: %s, body size: %d", boundary, len(body))

	partCount := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			log.Debugf("[observerbuffer] Reached EOF after %d parts, no 'event' field found", partCount)
			break
		}
		if err != nil {
			log.Debugf("[observerbuffer] Failed to read multipart part: %v", err)
			return nil
		}
		partCount++

		formName := part.FormName()
		log.Debugf("[observerbuffer] Found multipart part #%d: '%s'", partCount, formName)

		// Look for the "event" field which contains JSON metadata
		if formName == "event" {
			data, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				log.Debugf("Failed to read event field: %v", err)
				return nil
			}

			// Parse the JSON event data
			var event struct {
				Version     string   `json:"version"`
				Family      string   `json:"family"`        // profile type: cpu, heap, etc.
				Tags        string   `json:"tags_profiler"` // comma-separated string
				Start       string   `json:"start"`
				End         string   `json:"end"`
				Attachments []string `json:"attachments"`
			}

			if err := json.Unmarshal(data, &event); err != nil {
				log.Debugf("Failed to unmarshal profile event JSON: %v", err)
				return nil
			}

			metadata := &profileMetadata{
				ProfileType: event.Family,
				Version:     event.Version,
				Tags:        make(map[string]string),
			}

			// Parse tags - tags_profiler is a comma-separated string like "service:myapp,env:prod,host:hostname"
			if event.Tags != "" {
				tagParts := bytes.Split([]byte(event.Tags), []byte(","))
				for _, tagPart := range tagParts {
					// Each tag is in format "key:value"
					if idx := bytes.IndexByte(tagPart, ':'); idx > 0 {
						key := string(bytes.TrimSpace(tagPart[:idx]))
						value := string(bytes.TrimSpace(tagPart[idx+1:]))

						switch key {
						case "service":
							metadata.Service = value
						case "env":
							metadata.Env = value
						case "host":
							metadata.Hostname = value
						case "profile_id":
							metadata.ProfileID = value
						default:
							metadata.Tags[key] = value
						}
					}
				}
			}

			// Parse start time and calculate duration from start/end if available
			if event.Start != "" {
				// Timestamps are typically in RFC3339 format
				if startTime, err := time.Parse(time.RFC3339Nano, event.Start); err == nil {
					metadata.TimestampNs = startTime.UnixNano()

					if event.End != "" {
						if endTime, err := time.Parse(time.RFC3339Nano, event.End); err == nil {
							metadata.DurationNs = endTime.Sub(startTime).Nanoseconds()
						}
					}
				}
			}

			return metadata
		}
		part.Close()
	}

	return nil
}

// AddStats adds a stats payload to the buffer.
func (b *bufferImpl) AddStats(payload *pb.StatsPayload) {
	if payload == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// If buffer is full, drop the oldest entry
	if len(b.statsBuffer) >= b.statsCap {
		// Shift buffer left by one (drop oldest)
		copy(b.statsBuffer, b.statsBuffer[1:])
		b.statsBuffer = b.statsBuffer[:len(b.statsBuffer)-1]
		b.statsDropped.Add(1)
		b.statsDroppedSinceDrain++
	}

	b.statsBuffer = append(b.statsBuffer, observerbuffer.BufferedStats{
		Payload:      payload,
		ReceivedAtNs: time.Now().UnixNano(),
	})
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

// DrainStats removes and returns all buffered stats payloads.
func (b *bufferImpl) DrainStats() (stats []observerbuffer.BufferedStats, droppedCount uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	droppedCount = b.statsDroppedSinceDrain
	b.statsDroppedSinceDrain = 0

	if len(b.statsBuffer) == 0 {
		return nil, droppedCount
	}

	// Copy the stats to return
	stats = make([]observerbuffer.BufferedStats, len(b.statsBuffer))
	copy(stats, b.statsBuffer)

	// Clear the buffer
	b.statsBuffer = b.statsBuffer[:0]

	return stats, droppedCount
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
		StatsCount:      len(b.statsBuffer),
		StatsCapacity:   b.statsCap,
		StatsDropped:    b.statsDropped.Load(),
	}
}

// noopBuffer is a no-op implementation when buffering is disabled.
type noopBuffer struct{}

func (n *noopBuffer) AddTrace(_ *pb.TracerPayload)                  {}
func (n *noopBuffer) AddProfile(_ observerbuffer.ProfileData)       {}
func (n *noopBuffer) AddRawProfile(_ []byte, _ map[string][]string) {}
func (n *noopBuffer) AddStats(_ *pb.StatsPayload)                   {}

func (n *noopBuffer) DrainTraces(_ uint32) ([]observerbuffer.BufferedTrace, uint64, bool) {
	return nil, 0, false
}

func (n *noopBuffer) DrainProfiles(_ uint32) ([]observerbuffer.ProfileData, uint64, bool) {
	return nil, 0, false
}

func (n *noopBuffer) DrainStats() ([]observerbuffer.BufferedStats, uint64) {
	return nil, 0
}

func (n *noopBuffer) Stats() observerbuffer.BufferStats {
	return observerbuffer.BufferStats{}
}
