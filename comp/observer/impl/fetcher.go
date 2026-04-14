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

// observerFetcher periodically fetches traces, trace stats, and profiles from
// remote trace-agents using the remoteAgentRegistry's observer methods.
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

// runTraceFetcher periodically fetches traces and trace stats from all registered trace-agents.
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

		for _, traceData := range result.Traces {
			if len(traceData.PayloadData) == 0 {
				continue
			}

			var payload pb.TracerPayload
			if _, err := payload.UnmarshalMsg(traceData.PayloadData); err != nil {
				pkglog.Warnf("[observer] failed to unmarshal trace payload: %v", err)
				continue
			}
			f.handle.ObserveTrace(&tracerPayloadView{payload: &payload, receivedAt: traceData.ReceivedAtNs})
		}

		for _, statsBytes := range result.StatsPayloads {
			if len(statsBytes) == 0 {
				continue
			}

			var statsPayload pb.StatsPayload
			if _, err := statsPayload.UnmarshalMsg(statsBytes); err != nil {
				pkglog.Warnf("[observer] failed to unmarshal stats payload: %v", err)
				continue
			}
			f.handle.ObserveTraceStats(&statsPayloadView{payload: &statsPayload})
		}

		if result.HasMore {
			hasMore = true
		}
	}

	// If there's more data, immediately fetch again.
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
	if len(v.payload.Chunks) > 0 && len(v.payload.Chunks[0].Spans) > 0 {
		span := v.payload.Chunks[0].Spans[0]
		return 0, span.TraceID
	}
	return 0, 0
}

func (v *tracerPayloadView) GetSpans() observerdef.SpanIterator {
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

func (v *tracerPayloadView) GetEnv() string              { return v.payload.Env }
func (v *tracerPayloadView) GetService() string          { return "" }
func (v *tracerPayloadView) GetHostname() string         { return v.payload.Hostname }
func (v *tracerPayloadView) GetContainerID() string      { return v.payload.ContainerID }
func (v *tracerPayloadView) GetTimestampUnixNano() int64 { return v.receivedAt }
func (v *tracerPayloadView) GetDurationNano() int64      { return 0 }
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

// statsPayloadView adapts a *pb.StatsPayload to the TraceStatsView interface.
type statsPayloadView struct {
	payload *pb.StatsPayload
}

func (v *statsPayloadView) GetAgentHostname() string { return v.payload.AgentHostname }
func (v *statsPayloadView) GetAgentEnv() string      { return v.payload.AgentEnv }
func (v *statsPayloadView) GetRows() observerdef.TraceStatsRowIterator {
	return &statsRowIterator{payload: v.payload, clientIdx: 0, bucketIdx: 0, groupIdx: -1}
}

// statsRowIterator iterates over denormalized rows of a statsPayloadView.
type statsRowIterator struct {
	payload   *pb.StatsPayload
	clientIdx int
	bucketIdx int
	groupIdx  int
	current   *statsRowView
}

func (it *statsRowIterator) Next() bool {
	for it.clientIdx < len(it.payload.Stats) {
		client := it.payload.Stats[it.clientIdx]
		for it.bucketIdx < len(client.Stats) {
			bucket := client.Stats[it.bucketIdx]
			it.groupIdx++
			if it.groupIdx < len(bucket.Stats) {
				it.current = &statsRowView{
					client: client,
					bucket: bucket,
					group:  bucket.Stats[it.groupIdx],
				}
				return true
			}
			it.bucketIdx++
			it.groupIdx = -1
		}
		it.clientIdx++
		it.bucketIdx = 0
		it.groupIdx = -1
	}
	return false
}

func (it *statsRowIterator) Row() observerdef.TraceStatRow { return it.current }

// statsRowView adapts a denormalized stats row to the TraceStatRow interface.
type statsRowView struct {
	client *pb.ClientStatsPayload
	bucket *pb.ClientStatsBucket
	group  *pb.ClientGroupedStats
}

func (r *statsRowView) GetClientHostname() string      { return r.client.Hostname }
func (r *statsRowView) GetClientEnv() string           { return r.client.Env }
func (r *statsRowView) GetClientVersion() string       { return r.client.Version }
func (r *statsRowView) GetClientContainerID() string   { return r.client.ContainerID }
func (r *statsRowView) GetBucketStartUnixNano() uint64 { return r.bucket.Start }
func (r *statsRowView) GetBucketDurationNano() uint64  { return r.bucket.Duration }
func (r *statsRowView) GetService() string             { return r.group.Service }
func (r *statsRowView) GetName() string                { return r.group.Name }
func (r *statsRowView) GetResource() string            { return r.group.Resource }
func (r *statsRowView) GetType() string                { return r.group.Type }
func (r *statsRowView) GetHTTPStatusCode() uint32      { return r.group.HTTPStatusCode }
func (r *statsRowView) GetSpanKind() string            { return r.group.SpanKind }
func (r *statsRowView) GetIsTraceRoot() int32          { return int32(r.group.IsTraceRoot) }
func (r *statsRowView) GetSynthetics() bool            { return r.group.Synthetics }
func (r *statsRowView) GetHits() uint64                { return r.group.Hits }
func (r *statsRowView) GetErrors() uint64              { return r.group.Errors }
func (r *statsRowView) GetTopLevelHits() uint64        { return r.group.TopLevelHits }
func (r *statsRowView) GetDurationNano() uint64        { return r.group.Duration }
func (r *statsRowView) GetOkSummary() []byte           { return r.group.OkSummary }
func (r *statsRowView) GetErrorSummary() []byte        { return r.group.ErrorSummary }
func (r *statsRowView) GetPeerTags() []string          { return r.group.PeerTags }

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
func (v *spanView) GetStartUnixNano() int64        { return v.span.Start }
func (v *spanView) GetDurationNano() int64         { return v.span.Duration }
func (v *spanView) GetError() int32                { return v.span.Error }
func (v *spanView) GetMeta() map[string]string     { return v.span.Meta }
func (v *spanView) GetMetrics() map[string]float64 { return v.span.Metrics }

// profileDataView adapts ProfileData proto to the ProfileView interface.
type profileDataView struct {
	data *pbcore.ProfileData
}

func (v *profileDataView) GetProfileID() string        { return v.data.ProfileId }
func (v *profileDataView) GetProfileType() string      { return v.data.ProfileType }
func (v *profileDataView) GetService() string          { return v.data.Service }
func (v *profileDataView) GetEnv() string              { return v.data.Env }
func (v *profileDataView) GetVersion() string          { return v.data.Version }
func (v *profileDataView) GetHostname() string         { return v.data.Hostname }
func (v *profileDataView) GetContainerID() string      { return v.data.ContainerId }
func (v *profileDataView) GetTimestampUnixNano() int64 { return v.data.TimestampNs }
func (v *profileDataView) GetDurationNano() int64      { return v.data.DurationNs }
func (v *profileDataView) GetTags() map[string]string  { return v.data.Tags }
func (v *profileDataView) GetContentType() string      { return v.data.ContentType }
func (v *profileDataView) GetRawData() []byte          { return v.data.InlineData }
func (v *profileDataView) GetExternalPath() string     { return "" }
