// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"go.uber.org/atomic"
)

// SpanConcentratorConfig exposes configuration options for a SpanConcentrator
type SpanConcentratorConfig struct {
	// ComputeStatsBySpanKind enables/disables the computing of stats based on a span's `span.kind` field
	ComputeStatsBySpanKind bool
	// BucketInterval the size of our pre-aggregation per bucket
	BucketInterval int64

	// The fields below are tracer-only controls (dd-trace-go imports this package and sets them).
	// The Agent intentionally leaves them all at zero so all cardinality caps are no-ops in the Agent.

	// AdditionalMetricTagsCardinalityLimit caps distinct additional_metric_tags entries per bucket. 0 = no cap.
	AdditionalMetricTagsCardinalityLimit int
	// ResourceCardinalityLimit caps distinct resource values per bucket. 0 = no cap.
	ResourceCardinalityLimit int
	// HTTPEndpointCardinalityLimit caps distinct http_endpoint values per bucket. 0 = no cap.
	HTTPEndpointCardinalityLimit int
	// PeerTagsCardinalityLimit caps distinct peer_tags combinations per bucket. 0 = no cap.
	PeerTagsCardinalityLimit int
	// OriginCardinalityLimit caps distinct origin values per bucket. 0 = no cap.
	OriginCardinalityLimit int
	// WholeKeyCardinalityLimit caps the total distinct BucketsAggregationKeys per bucket. 0 = no cap.
	// This is the backstop that guarantees a hard memory bound regardless of which field causes explosion.
	WholeKeyCardinalityLimit int

	// ObfuscationEnabled signals that the tracer is performing obfuscation/normalization.
	// When true, string length caps (service ≤ 100, name ≤ 100, type ≤ 100, resource ≤ ResourceMaxBytes) are applied.
	ObfuscationEnabled bool
	// ResourceMaxBytes is the max byte length for resource strings when ObfuscationEnabled is true.
	// Defaults to 5000; tracers should set to 15000 when the agent /info endpoint advertises the big_resource feature flag.
	ResourceMaxBytes int
}

// StatSpan holds all the required fields from a span needed to calculate stats
type StatSpan struct {
	service  string
	resource string
	name     string
	typ      string
	error    int32
	parentID uint64
	start    int64
	duration int64

	//Fields below this are derived on creation

	spanKind                     string
	serviceSource                string
	statusCode                   uint32
	isTopLevel                   bool
	matchingPeerTags             []string
	matchingAdditionalMetricTags []string
	grpcStatusCode               string

	httpMethod   string
	httpEndpoint string
}

const (
	// additionalMetricTagValueMaxLength bounds an individual tag value; longer values are
	// masked. This length cap is unconditional and runs in BOTH the Agent and the tracer.
	additionalMetricTagValueMaxLength = 200
	// Masked values carry a sentinel naming the component that masked them. The sentinel is
	// propagated into each RawBucket so the same masking code attributes correctly in either
	// context: tracer-side masking reports tracer_blocked_value, Agent-side reports agent_blocked_value.
	blockedByTracerSentinel = "tracer_blocked_value"
	blockedByAgentSentinel  = "agent_blocked_value"

	// defaultResourceMaxBytes is the resource string length cap when big_resource is not advertised by the agent.
	defaultResourceMaxBytes = 5000
	// bigResourceMaxBytes is used when the agent /info endpoint advertises the big_resource feature flag.
	bigResourceMaxBytes = 15000
	// stringFieldMaxBytes is the length cap for service, name, and type fields.
	stringFieldMaxBytes = 100
)

func maskAdditionalMetricTagValues(tags []string, sentinel string) []string {
	if sentinel == "" {
		sentinel = blockedByTracerSentinel
	}
	masked := make([]string, 0, len(tags))
	for _, t := range tags {
		k, _, _ := strings.Cut(t, ":")
		masked = append(masked, k+":"+sentinel)
	}
	return masked
}

func matchingPeerTags(meta map[string]string, peerTagKeys []string) []string {
	if len(peerTagKeys) == 0 {
		return nil
	}
	reg := semantics.DefaultRegistry()
	a := semantics.NewStringMapAccessor(meta)
	spanKind := semantics.LookupString(reg, a, semantics.ConceptSpanKind)
	baseService := semantics.LookupString(reg, a, semantics.ConceptDDBaseService)
	var pt []string
	for _, t := range peerTagKeysToAggregateForSpan(spanKind, baseService, peerTagKeys) {
		if v, ok := meta[t]; ok && v != "" {
			v = obfuscate.QuantizePeerIPAddresses(v)
			pt = append(pt, t+":"+v)
		}
	}
	return pt
}

func matchingPeerTagsV1(s *idx.InternalSpan, peerTagKeys []string) []string {
	if len(peerTagKeys) == 0 {
		return nil
	}
	a := semantics.NewDDSpanAccessorV1(s)
	baseService := semantics.LookupString(semantics.DefaultRegistry(), a, semantics.ConceptDDBaseService)
	var pt []string
	for _, t := range peerTagKeysToAggregateForSpan(s.SpanKind(), baseService, peerTagKeys) {
		if v, ok := s.GetAttributeAsString(t); ok && v != "" {
			v = obfuscate.QuantizePeerIPAddresses(v)
			pt = append(pt, t+":"+v)
		}
	}
	return pt
}

// peerTagKeysToAggregateForSpan returns the set of peerTagKeys to use for stats aggregation for the given
// span.kind and _dd.base_service
func peerTagKeysToAggregateForSpan(spanKind string, baseService string, peerTagKeys []string) []string {
	if len(peerTagKeys) == 0 {
		return nil
	}
	spanKind = strings.ToLower(spanKind)
	if (spanKind == "" || spanKind == "internal") && baseService != "" {
		// it's a service override on an internal span so it comes from custom instrumentation and does not represent
		// a client|producer|consumer span which is talking to a peer entity
		// in this case only the base service tag is relevant for stats aggregation
		return []string{string(semantics.ConceptDDBaseService)}
	}
	if spanKind == "client" || spanKind == "producer" || spanKind == "consumer" {
		return peerTagKeys
	}
	return nil
}

func (sc *SpanConcentrator) matchingAdditionalMetricTags(meta map[string]string, additionalMetricTagKeys []string) []string {
	if len(additionalMetricTagKeys) == 0 {
		return nil
	}
	var tags []string
	for _, t := range additionalMetricTagKeys {
		if v, ok := meta[t]; ok && v != "" {
			if len(v) > additionalMetricTagValueMaxLength {
				v = sc.getAdditionalMetricTagValueBlockSentinel()
				sc.addLengthBlock()
			}
			tags = append(tags, t+":"+v)
		}
	}
	return tags
}

func (sc *SpanConcentrator) matchingAdditionalMetricTagsV1(s *idx.InternalSpan, additionalMetricTagKeys []string) []string {
	if len(additionalMetricTagKeys) == 0 {
		return nil
	}
	var tags []string
	for _, t := range additionalMetricTagKeys {
		if v, ok := s.GetAttributeAsString(t); ok && v != "" {
			if len(v) > additionalMetricTagValueMaxLength {
				v = sc.getAdditionalMetricTagValueBlockSentinel()
				sc.addLengthBlock()
			}
			tags = append(tags, t+":"+v)
		}
	}
	return tags
}

// SpanConcentrator produces time bucketed statistics from a stream of raw spans.
type SpanConcentrator struct {
	computeStatsBySpanKind                bool
	additionalMetricTagValueBlockSentinel string
	cardinalityLimits                     BucketCardinalityLimits
	obfuscationEnabled                    *atomic.Bool  // nil when zero-value; read via isObfuscationEnabled()
	resourceMaxBytes                      *atomic.Int64 // nil when zero-value; read via getResourceMaxBytes()

	// per-collapse-type atomic counters; drained on each flush via DrainBlockCounts
	lengthBlocks          *atomic.Int64 // additional_metric_tags value > 200 chars
	capBlocks             *atomic.Int64 // additional_metric_tags cardinality cap
	wholeKeyCollapses     *atomic.Int64
	resourceCollapses     *atomic.Int64
	httpEndpointCollapses *atomic.Int64
	peerTagsCollapses     *atomic.Int64
	originCollapses       *atomic.Int64

	// bucket duration in nanoseconds
	bsize int64
	// Timestamp of the oldest time bucket for which we allow data.
	// Any ingested stats older than it get added to this bucket.
	oldestTs int64
	// bufferLen is the number of 10s stats bucket we keep in memory before flushing them.
	// It means that we can compute stats only for the last `bufferLen * bsize` and that we
	// wait such time before flushing the stats.
	// This only applies to past buckets. Stats buckets in the future are allowed with no restriction.
	bufferLen int

	// mu protects the buckets field
	mu      sync.Mutex
	buckets map[int64]*RawBucket
}

// NewSpanConcentrator builds a new SpanConcentrator object
func NewSpanConcentrator(cfg *SpanConcentratorConfig, now time.Time) *SpanConcentrator {
	resourceMaxBytes := cfg.ResourceMaxBytes
	if resourceMaxBytes <= 0 {
		resourceMaxBytes = 5000
	}
	sc := &SpanConcentrator{
		computeStatsBySpanKind:                cfg.ComputeStatsBySpanKind,
		additionalMetricTagValueBlockSentinel: blockedByTracerSentinel,
		cardinalityLimits: BucketCardinalityLimits{
			AdditionalTags: cfg.AdditionalMetricTagsCardinalityLimit,
			Resource:       cfg.ResourceCardinalityLimit,
			HTTPEndpoint:   cfg.HTTPEndpointCardinalityLimit,
			PeerTags:       cfg.PeerTagsCardinalityLimit,
			Origin:         cfg.OriginCardinalityLimit,
			WholeKey:       cfg.WholeKeyCardinalityLimit,
		},
		obfuscationEnabled:    atomic.NewBool(cfg.ObfuscationEnabled),
		resourceMaxBytes:      atomic.NewInt64(int64(resourceMaxBytes)),
		lengthBlocks:          atomic.NewInt64(0),
		capBlocks:             atomic.NewInt64(0),
		wholeKeyCollapses:     atomic.NewInt64(0),
		resourceCollapses:     atomic.NewInt64(0),
		httpEndpointCollapses: atomic.NewInt64(0),
		peerTagsCollapses:     atomic.NewInt64(0),
		originCollapses:       atomic.NewInt64(0),
		bsize:                 cfg.BucketInterval,
		oldestTs:              alignTs(now.UnixNano(), cfg.BucketInterval),
		bufferLen:             defaultBufferLen,
		mu:                    sync.Mutex{},
		buckets:               make(map[int64]*RawBucket),
	}
	return sc
}

func (sc *SpanConcentrator) getAdditionalMetricTagValueBlockSentinel() string {
	if sc.additionalMetricTagValueBlockSentinel == "" {
		return blockedByTracerSentinel
	}
	return sc.additionalMetricTagValueBlockSentinel
}

func (sc *SpanConcentrator) isObfuscationEnabled() bool {
	return sc.obfuscationEnabled != nil && sc.obfuscationEnabled.Load()
}

func (sc *SpanConcentrator) getResourceMaxBytes() int {
	if sc.resourceMaxBytes == nil {
		return defaultResourceMaxBytes
	}
	return int(sc.resourceMaxBytes.Load())
}

// SetObfuscationEnabled updates whether string length caps are applied during stat span creation.
// Should be called when the agent's obfuscation version is known (e.g. after /info is fetched).
// bigResource should be true when the agent's /info feature_flags contains "big_resource".
func (sc *SpanConcentrator) SetObfuscationEnabled(enabled bool, bigResource bool) {
	if sc.obfuscationEnabled == nil || sc.resourceMaxBytes == nil {
		return
	}
	sc.obfuscationEnabled.Store(enabled)
	if enabled {
		if bigResource {
			sc.resourceMaxBytes.Store(bigResourceMaxBytes)
		} else {
			sc.resourceMaxBytes.Store(defaultResourceMaxBytes)
		}
	}
}

func (sc *SpanConcentrator) addLengthBlock() {
	if sc.lengthBlocks != nil {
		sc.lengthBlocks.Add(1)
	}
}

func (sc *SpanConcentrator) addCollapses(result SpanCollapseResult) {
	if result.AdditionalTagsCapBlock && sc.capBlocks != nil {
		sc.capBlocks.Add(1)
	}
	if result.WholeKeyCollapsed && sc.wholeKeyCollapses != nil {
		sc.wholeKeyCollapses.Add(1)
	}
	if result.ResourceCollapsed && sc.resourceCollapses != nil {
		sc.resourceCollapses.Add(1)
	}
	if result.HTTPEndpointCollapsed && sc.httpEndpointCollapses != nil {
		sc.httpEndpointCollapses.Add(1)
	}
	if result.PeerTagsCollapsed && sc.peerTagsCollapses != nil {
		sc.peerTagsCollapses.Add(1)
	}
	if result.OriginCollapsed && sc.originCollapses != nil {
		sc.originCollapses.Add(1)
	}
}

// BlockCounts reports cardinality collapse events since the last drain.
type BlockCounts struct {
	// LengthBlocks counts additional_metric_tags values that exceeded the 200-char length cap.
	LengthBlocks int64
	// CapBlocks counts additional_metric_tags entries that exceeded the per-bucket cardinality cap.
	CapBlocks int64
	// WholeKeyCollapses counts spans collapsed due to the whole-key cardinality limit.
	WholeKeyCollapses int64
	// ResourceCollapses counts spans whose resource field was collapsed to the sentinel.
	ResourceCollapses int64
	// HTTPEndpointCollapses counts spans whose http_endpoint field was collapsed to the sentinel.
	HTTPEndpointCollapses int64
	// PeerTagsCollapses counts spans whose peer_tags were collapsed to the sentinel.
	PeerTagsCollapses int64
	// OriginCollapses counts spans whose origin was collapsed.
	OriginCollapses int64
}

// DrainBlockCounts atomically reads and zeroes all collapse counters.
func (sc *SpanConcentrator) DrainBlockCounts() BlockCounts {
	var counts BlockCounts
	if sc.lengthBlocks != nil {
		counts.LengthBlocks = sc.lengthBlocks.Swap(0)
	}
	if sc.capBlocks != nil {
		counts.CapBlocks = sc.capBlocks.Swap(0)
	}
	if sc.wholeKeyCollapses != nil {
		counts.WholeKeyCollapses = sc.wholeKeyCollapses.Swap(0)
	}
	if sc.resourceCollapses != nil {
		counts.ResourceCollapses = sc.resourceCollapses.Swap(0)
	}
	if sc.httpEndpointCollapses != nil {
		counts.HTTPEndpointCollapses = sc.httpEndpointCollapses.Swap(0)
	}
	if sc.peerTagsCollapses != nil {
		counts.PeerTagsCollapses = sc.peerTagsCollapses.Swap(0)
	}
	if sc.originCollapses != nil {
		counts.OriginCollapses = sc.originCollapses.Swap(0)
	}
	return counts
}

// NewStatSpanFromPB is a helper version of NewStatSpanWithConfig that builds a StatSpan from a pb.Span.
func (sc *SpanConcentrator) NewStatSpanFromPB(s *pb.Span, peerTags []string, additionalMetricTagKeys []string) (statSpan *StatSpan, ok bool) {
	return sc.NewStatSpanWithConfig(
		StatSpanConfig{
			Service:                 s.Service,
			Resource:                s.Resource,
			Name:                    s.Name,
			Type:                    s.Type,
			ParentID:                s.ParentID,
			Start:                   s.Start,
			Duration:                s.Duration,
			Error:                   s.Error,
			Meta:                    s.Meta,
			Metrics:                 s.Metrics,
			PeerTags:                peerTags,
			AdditionalMetricTagKeys: additionalMetricTagKeys,
			HTTPMethod:              "",
			HTTPEndpoint:            "",
		},
	)
}

// StatSpanConfig holds the configuration options for creating a StatSpan using NewStatSpanWithConfig
type StatSpanConfig struct {
	Service                 string
	Resource                string
	Name                    string
	Type                    string
	ParentID                uint64
	Start                   int64
	Duration                int64
	Error                   int32
	Meta                    map[string]string
	Metrics                 map[string]float64
	PeerTags                []string
	AdditionalMetricTagKeys []string
	HTTPMethod              string
	HTTPEndpoint            string
}

// NewStatSpanWithConfig builds a StatSpan from the required fields for stats calculation
// peerTags is the configured list of peer tags to look for
// returns (nil,false) if the provided fields indicate a span should not have stats calculated
func (sc *SpanConcentrator) NewStatSpanWithConfig(config StatSpanConfig) (statSpan *StatSpan, ok bool) {
	if config.Meta == nil {
		config.Meta = make(map[string]string)
	}
	if config.Metrics == nil {
		config.Metrics = make(map[string]float64)
	}
	a := semantics.NewDDSpanAccessor(config.Meta, config.Metrics)
	spanKind := semantics.LookupString(semantics.DefaultRegistry(), a, semantics.ConceptSpanKind)
	eligibleSpanKind := sc.computeStatsBySpanKind && computeStatsForSpanKind(spanKind)
	isTopLevel := traceutil.HasTopLevelMetrics(config.Metrics)
	if !(isTopLevel || traceutil.IsMeasuredMetrics(config.Metrics) || eligibleSpanKind) {
		return nil, false
	}
	if traceutil.IsPartialSnapshotMetrics(config.Metrics) {
		return nil, false
	}
	service, name, typ, resource := config.Service, config.Name, config.Type, config.Resource
	if sc.isObfuscationEnabled() {
		resourceMax := sc.getResourceMaxBytes()
		service = normalize.TruncateUTF8(service, stringFieldMaxBytes)
		name = normalize.TruncateUTF8(name, stringFieldMaxBytes)
		typ = normalize.TruncateUTF8(typ, stringFieldMaxBytes)
		resource = normalize.TruncateUTF8(resource, resourceMax)
	}
	return &StatSpan{
		service:                      service,
		resource:                     resource,
		name:                         name,
		typ:                          typ,
		error:                        config.Error,
		parentID:                     config.ParentID,
		start:                        config.Start,
		duration:                     config.Duration,
		spanKind:                     spanKind,
		serviceSource:                config.Meta[tagServiceSource],
		statusCode:                   getStatusCode(config.Meta, config.Metrics),
		isTopLevel:                   isTopLevel,
		matchingPeerTags:             matchingPeerTags(config.Meta, config.PeerTags),
		matchingAdditionalMetricTags: sc.matchingAdditionalMetricTags(config.Meta, config.AdditionalMetricTagKeys),

		grpcStatusCode: getGRPCStatusCode(config.Meta, config.Metrics),

		httpMethod:   config.HTTPMethod,
		httpEndpoint: config.HTTPEndpoint,
	}, true
}

// NewStatSpanFromV1 is a helper version of NewStatSpan that builds a StatSpan from an idx.InternalSpan.
func (sc *SpanConcentrator) NewStatSpanFromV1(s *idx.InternalSpan, peerTags []string, additionalMetricTagKeys []string) (statSpan *StatSpan, ok bool) {
	eligibleSpanKind := sc.computeStatsBySpanKind && computeStatsForSpanKindV1(s.Kind())
	isTopLevel := traceutil.HasTopLevelMetricsV1(s)
	if !(isTopLevel || traceutil.IsMeasuredMetricsV1(s) || eligibleSpanKind) {
		return nil, false
	}
	if traceutil.IsPartialSnapshotMetricsV1(s) {
		return nil, false
	}
	spanError := 0
	if s.Error() {
		spanError = 1
	}
	serviceSource, _ := s.GetAttributeAsString(tagServiceSource)
	service, name, typ, resource := s.Service(), s.Name(), s.Type(), s.Resource()
	if sc.isObfuscationEnabled() {
		resourceMax := sc.getResourceMaxBytes()
		service = normalize.TruncateUTF8(service, stringFieldMaxBytes)
		name = normalize.TruncateUTF8(name, stringFieldMaxBytes)
		typ = normalize.TruncateUTF8(typ, stringFieldMaxBytes)
		resource = normalize.TruncateUTF8(resource, resourceMax)
	}
	return &StatSpan{
		service:                      service,
		resource:                     resource,
		name:                         name,
		typ:                          typ,
		error:                        int32(spanError),
		parentID:                     s.ParentID(),
		start:                        int64(s.Start()),
		duration:                     int64(s.Duration()),
		spanKind:                     s.SpanKind(),
		serviceSource:                serviceSource,
		statusCode:                   getStatusCodeV1(s),
		isTopLevel:                   isTopLevel,
		matchingPeerTags:             matchingPeerTagsV1(s, peerTags),
		matchingAdditionalMetricTags: sc.matchingAdditionalMetricTagsV1(s, additionalMetricTagKeys),
		grpcStatusCode:               getGRPCStatusCodeV1(s),
	}, true
}

// NewStatSpan builds a StatSpan from the required fields for stats calculation
// peerTags is the configured list of peer tags to look for
// returns (nil,false) if the provided fields indicate a span should not have stats calculated
// Deprecated: use NewStatSpanWithConfig instead
func (sc *SpanConcentrator) NewStatSpan(
	service, resource, name string,
	typ string,
	parentID uint64,
	start, duration int64,
	error int32,
	meta map[string]string,
	metrics map[string]float64,
	peerTags []string,
) (statSpan *StatSpan, ok bool) {
	return sc.NewStatSpanWithConfig(
		StatSpanConfig{
			Service:                 service,
			Resource:                resource,
			Name:                    name,
			Type:                    typ,
			ParentID:                parentID,
			Start:                   start,
			Duration:                duration,
			Error:                   error,
			Meta:                    meta,
			Metrics:                 metrics,
			PeerTags:                peerTags,
			AdditionalMetricTagKeys: nil,
			HTTPMethod:              "",
			HTTPEndpoint:            "",
		},
	)
}

// computeStatsForSpanKind returns true if the span.kind value makes the span eligible for stats computation.
func computeStatsForSpanKind(kind string) bool {
	k := strings.ToLower(kind)
	_, ok := KindsComputed[k]
	return ok
}

// computeStatsForSpanKindV1 returns true if the span.kind value makes the span eligible for stats computation.
func computeStatsForSpanKindV1(kind idx.SpanKind) bool {
	// TODO: refactor this to avoid duplication here
	return kind == idx.SpanKind_SPAN_KIND_SERVER ||
		kind == idx.SpanKind_SPAN_KIND_CONSUMER ||
		kind == idx.SpanKind_SPAN_KIND_CLIENT ||
		kind == idx.SpanKind_SPAN_KIND_PRODUCER
}

// KindsComputed is the list of span kinds that will have stats computed on them
// when computeStatsByKind is enabled in the concentrator.
var KindsComputed = map[string]struct{}{
	"server":   {},
	"consumer": {},
	"client":   {},
	"producer": {},
}

func (sc *SpanConcentrator) addSpan(s *StatSpan, aggKey PayloadAggregationKey, tags infraTags, origin string, weight float64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	end := s.start + s.duration
	btime := max(end-end%sc.bsize, sc.oldestTs)

	b, ok := sc.buckets[btime]
	if !ok {
		b = NewRawBucket(uint64(btime), uint64(sc.bsize), sc.cardinalityLimits)
		b.additionalMetricTagValueBlockSentinel = sc.getAdditionalMetricTagValueBlockSentinel()
		sc.buckets[btime] = b
	}
	if tags.processTagsHash != 0 && len(tags.processTags) > 0 {
		b.processTagsByHash[tags.processTagsHash] = tags.processTags
	}
	if tags.containerID != "" && len(tags.containerTags) > 0 {
		b.containerTagsByID[tags.containerID] = tags.containerTags
	}
	result := b.HandleSpan(s, weight, origin, aggKey)
	sc.addCollapses(result)
}

// AddSpan to the SpanConcentrator, appending the new data to the appropriate internal bucket.
// todo:raphael migrate dd-trace-go API to not depend on containerID/containerTags and add processTags at encoding layer
func (sc *SpanConcentrator) AddSpan(s *StatSpan, aggKey PayloadAggregationKey, containerID string, containerTags []string, origin string) {
	sc.addSpan(s, aggKey, infraTags{containerID: containerID, containerTags: containerTags}, origin, 1)
}

// Flush deletes and returns complete ClientStatsPayloads.
// The force boolean guarantees flushing all buckets if set to true.
func (sc *SpanConcentrator) Flush(now int64, force bool) []*pb.ClientStatsPayload {
	m := make(map[PayloadAggregationKey][]*pb.ClientStatsBucket)
	containerTagsByID := make(map[string][]string)
	processTagsByHash := make(map[uint64]string)

	sc.mu.Lock()
	for ts, srb := range sc.buckets {
		// Always keep `bufferLen` buckets (default is 2: current + previous one).
		// This is a trade-off: we accept slightly late traces (clock skew and stuff)
		// but we delay flushing by at most `bufferLen` buckets.
		//
		// This delay might result in not flushing stats payload (data loss)
		// if the agent stops while the latest buckets aren't old enough to be flushed.
		// The "force" boolean skips the delay and flushes all buckets, typically on agent shutdown.
		if !force && ts > now-int64(sc.bufferLen)*sc.bsize {
			log.Tracef("Bucket %d is not old enough to be flushed, keeping it", ts)
			continue
		}
		log.Debugf("Flushing bucket %d", ts)
		for k, b := range srb.Export() {
			m[k] = append(m[k], b)
			if ctags, ok := srb.containerTagsByID[k.ContainerID]; ok {
				containerTagsByID[k.ContainerID] = ctags
			}
			if ptags, ok := srb.processTagsByHash[k.ProcessTagsHash]; ok {
				processTagsByHash[k.ProcessTagsHash] = ptags
			}
		}
		delete(sc.buckets, ts)
	}
	// After flushing, update the oldest timestamp allowed to prevent having stats for
	// an already-flushed bucket.
	newOldestTs := alignTs(now, sc.bsize) - int64(sc.bufferLen-1)*sc.bsize
	if newOldestTs > sc.oldestTs {
		log.Debugf("Update oldestTs to %d", newOldestTs)
		sc.oldestTs = newOldestTs
	}
	sc.mu.Unlock()
	sb := make([]*pb.ClientStatsPayload, 0, len(m))
	for k, s := range m {
		p := &pb.ClientStatsPayload{
			Env:             k.Env,
			Hostname:        k.Hostname,
			ContainerID:     k.ContainerID,
			Version:         k.Version,
			GitCommitSha:    k.GitCommitSha,
			ImageTag:        k.ImageTag,
			Lang:            k.Lang,
			Service:         k.BaseService,
			Stats:           s,
			Tags:            containerTagsByID[k.ContainerID],
			ProcessTags:     processTagsByHash[k.ProcessTagsHash],
			ProcessTagsHash: k.ProcessTagsHash,
		}
		sb = append(sb, p)
	}
	return sb
}
