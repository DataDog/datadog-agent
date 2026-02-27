// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"context"
	"sync"
	"time"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// FetcherConfig contains configuration for the observer fetcher.
type FetcherConfig struct {
	// TraceFetchInterval is how often to fetch traces from remote agents.
	TraceFetchInterval time.Duration
	// ProfileFetchInterval is how often to fetch profiles from remote agents.
	ProfileFetchInterval time.Duration
	// MaxTraceBatch is the maximum number of traces to fetch per request.
	MaxTraceBatch uint32
	// MaxProfileBatch is the maximum number of profiles to fetch per request.
	MaxProfileBatch uint32
}

// DefaultFetcherConfig returns the default fetcher configuration.
func DefaultFetcherConfig() FetcherConfig {
	return FetcherConfig{
		TraceFetchInterval:   5 * time.Second,
		ProfileFetchInterval: 10 * time.Second,
		MaxTraceBatch:        100,
		MaxProfileBatch:      50,
	}
}

// observerFetcher periodically fetches traces and profiles from remote trace-agents
// using the remoteAgentRegistry's GetObserverTraces and GetObserverProfiles methods.
type observerFetcher struct {
	registry remoteagentregistry.Component
	handle   observerdef.Handle
	config   FetcherConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// newObserverFetcher creates a new observer fetcher.
func newObserverFetcher(
	registry remoteagentregistry.Component,
	handle observerdef.Handle,
) *observerFetcher {
	return &observerFetcher{
		registry: registry,
		handle:   handle,
		config:   DefaultFetcherConfig(),
	}
}

// Start begins the periodic fetching goroutines.
func (f *observerFetcher) Start() {
	if f.registry == nil {
		pkglog.Debug("[observer] fetcher not started: no registry")
		return
	}

	f.ctx, f.cancel = context.WithCancel(context.Background())

	// Start trace fetcher
	f.wg.Add(1)
	go f.runTraceFetcher()

	// Start profile fetcher
	f.wg.Add(1)
	go f.runProfileFetcher()

	pkglog.Info("[observer] fetcher started")
}

// Stop stops the fetcher.
func (f *observerFetcher) Stop() {
	if f.cancel != nil {
		f.cancel()
	}
	f.wg.Wait()
	pkglog.Info("[observer] fetcher stopped")
}

// runTraceFetcher periodically fetches traces from all registered trace-agents.
func (f *observerFetcher) runTraceFetcher() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.config.TraceFetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			f.fetchTraces()
		}
	}
}

// runProfileFetcher periodically fetches profiles from all registered trace-agents.
func (f *observerFetcher) runProfileFetcher() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.config.ProfileFetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			f.fetchProfiles()
		}
	}
}

// fetchTraces fetches traces and stats from all registered trace-agents using the registry.
func (f *observerFetcher) fetchTraces() {
	results := f.registry.GetObserverTraces(f.config.MaxTraceBatch)

	hasMore := false
	for _, result := range results {
		if result.FailureReason != "" {
			pkglog.Warnf("[observer] failed to fetch traces from %s: %s", result.DisplayName, result.FailureReason)
			continue
		}

		if result.DroppedCount > 0 {
			pkglog.Warnf("[observer] %d traces were dropped in %s buffer", result.DroppedCount, result.DisplayName)
		}

		if result.StatsDroppedCount > 0 {
			pkglog.Warnf("[observer] %d stats payloads were dropped in %s buffer", result.StatsDroppedCount, result.DisplayName)
		}

		// Process traces
		for _, traceData := range result.Traces {
			if len(traceData.PayloadData) > 0 {
				// Deserialize the msgpack-encoded TracerPayload
				var payload pb.TracerPayload
				if _, err := payload.UnmarshalMsg(traceData.PayloadData); err != nil {
					pkglog.Warnf("[observer] failed to unmarshal trace payload: %v", err)
					continue
				}
				f.handle.ObserveTrace(&tracerPayloadView{payload: &payload, receivedAt: traceData.ReceivedAtNs})
			}
		}

		// Process stats payloads as metrics
		for _, statsBytes := range result.StatsPayloads {
			if len(statsBytes) == 0 {
				continue
			}
			var statsPayload pb.StatsPayload
			if _, err := statsPayload.UnmarshalMsg(statsBytes); err != nil {
				pkglog.Warnf("[observer] failed to unmarshal stats payload: %v", err)
				continue
			}
			processStatsPayload(f.handle, &statsPayload)
		}

		if result.HasMore {
			hasMore = true
		}
	}

	// If there's more data, immediately fetch again
	if hasMore {
		go f.fetchTraces()
	}
}

// fetchProfiles fetches profiles from all registered trace-agents using the registry.
func (f *observerFetcher) fetchProfiles() {
	results := f.registry.GetObserverProfiles(f.config.MaxProfileBatch)

	hasMore := false
	for _, result := range results {
		if result.FailureReason != "" {
			pkglog.Warnf("[observer] failed to fetch profiles from %s: %s", result.DisplayName, result.FailureReason)
			continue
		}

		if result.DroppedCount > 0 {
			pkglog.Warnf("[observer] %d profiles were dropped in %s buffer", result.DroppedCount, result.DisplayName)
		}

		for _, profileData := range result.Profiles {
			f.handle.ObserveProfile(&profileDataView{data: profileData})
		}

		if result.HasMore {
			hasMore = true
		}
	}

	// If there's more data, immediately fetch again
	if hasMore {
		go f.fetchProfiles()
	}
}

// tracerPayloadView adapts a TracerPayload to the TraceView interface.
type tracerPayloadView struct {
	payload    *pb.TracerPayload
	receivedAt int64
	spanIdx    int
	allSpans   []*pb.Span
}

func (v *tracerPayloadView) GetTraceID() (high, low uint64) {
	// TracerPayload doesn't have a single trace ID - it contains multiple chunks.
	// For the first chunk, return its trace ID.
	if len(v.payload.Chunks) > 0 && len(v.payload.Chunks[0].Spans) > 0 {
		span := v.payload.Chunks[0].Spans[0]
		return 0, span.TraceID // TraceID is 64-bit in the current format
	}
	return 0, 0
}

func (v *tracerPayloadView) GetSpans() observerdef.SpanIterator {
	// Flatten all spans from all chunks
	v.allSpans = nil
	for _, chunk := range v.payload.Chunks {
		v.allSpans = append(v.allSpans, chunk.Spans...)
	}
	v.spanIdx = -1
	return v
}

func (v *tracerPayloadView) Next() bool {
	v.spanIdx++
	return v.spanIdx < len(v.allSpans)
}

func (v *tracerPayloadView) Span() observerdef.SpanView {
	if v.spanIdx >= 0 && v.spanIdx < len(v.allSpans) {
		return &spanView{span: v.allSpans[v.spanIdx]}
	}
	return nil
}

func (v *tracerPayloadView) Reset() {
	v.spanIdx = -1
}

func (v *tracerPayloadView) GetEnv() string         { return v.payload.Env }
func (v *tracerPayloadView) GetService() string     { return "" } // Service is per-span
func (v *tracerPayloadView) GetHostname() string    { return v.payload.Hostname }
func (v *tracerPayloadView) GetContainerID() string { return v.payload.ContainerID }
func (v *tracerPayloadView) GetTimestamp() int64    { return v.receivedAt }
func (v *tracerPayloadView) GetDuration() int64     { return 0 } // Would need to calculate
func (v *tracerPayloadView) GetPriority() int32 {
	if len(v.payload.Chunks) > 0 {
		return v.payload.Chunks[0].Priority
	}
	return 0
}
func (v *tracerPayloadView) IsError() bool {
	for _, chunk := range v.payload.Chunks {
		for _, span := range chunk.Spans {
			if span.Error != 0 {
				return true
			}
		}
	}
	return false
}
func (v *tracerPayloadView) GetTags() map[string]string { return v.payload.Tags }

// spanView adapts a Span to the SpanView interface.
type spanView struct {
	span *pb.Span
}

func (v *spanView) GetSpanID() uint64              { return v.span.SpanID }
func (v *spanView) GetParentID() uint64            { return v.span.ParentID }
func (v *spanView) GetService() string             { return v.span.Service }
func (v *spanView) GetName() string                { return v.span.Name }
func (v *spanView) GetResource() string            { return v.span.Resource }
func (v *spanView) GetType() string                { return v.span.Type }
func (v *spanView) GetStart() int64                { return v.span.Start }
func (v *spanView) GetDuration() int64             { return v.span.Duration }
func (v *spanView) GetError() int32                { return v.span.Error }
func (v *spanView) GetMeta() map[string]string     { return v.span.Meta }
func (v *spanView) GetMetrics() map[string]float64 { return v.span.Metrics }

// profileDataView adapts ProfileData proto to the ProfileView interface.
type profileDataView struct {
	data *pbcore.ProfileData
}

func (v *profileDataView) GetProfileID() string       { return v.data.ProfileId }
func (v *profileDataView) GetProfileType() string     { return v.data.ProfileType }
func (v *profileDataView) GetService() string         { return v.data.Service }
func (v *profileDataView) GetEnv() string             { return v.data.Env }
func (v *profileDataView) GetVersion() string         { return v.data.Version }
func (v *profileDataView) GetHostname() string        { return v.data.Hostname }
func (v *profileDataView) GetContainerID() string     { return v.data.ContainerId }
func (v *profileDataView) GetTimestamp() int64        { return v.data.TimestampNs }
func (v *profileDataView) GetDuration() int64         { return v.data.DurationNs }
func (v *profileDataView) GetTags() map[string]string { return v.data.Tags }
func (v *profileDataView) GetContentType() string     { return v.data.ContentType }
func (v *profileDataView) GetRawData() []byte         { return v.data.InlineData }
func (v *profileDataView) GetExternalPath() string    { return "" }
