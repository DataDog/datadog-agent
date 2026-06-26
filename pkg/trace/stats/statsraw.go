// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"math"
	"math/rand"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/sketches-go/ddsketch"
)

const (
	// relativeAccuracy is the value accuracy we have on the percentiles. For example, we can
	// say that p99 is 100ms +- 1ms
	relativeAccuracy = 0.01
	// maxNumBins is the maximum number of bins of the ddSketch we use to store percentiles.
	// It can affect relative accuracy, but in practice, 2048 bins is enough to have 1% relative accuracy from
	// 80 micro second to 1 year: http://www.vldb.org/pvldb/vol12/p2195-masson.pdf
	maxNumBins = 2048
)

// Most "algorithm" stuff here is tested with stats_test.go as what is important
// is that the final data, the one with send after a call to Export(), is correct.

type groupedStats struct {
	// using float64 here to avoid the accumulation of rounding issues.
	hits                 float64
	topLevelHits         float64
	errors               float64
	duration             float64
	okDistribution       *ddsketch.DDSketch
	errDistribution      *ddsketch.DDSketch
	peerTags             []string
	additionalMetricTags []string
}

// round a float to an int, uniformly choosing
// between the lower and upper approximations.
func round(f float64) uint64 {
	i := uint64(f)
	if rand.Float64() < f-float64(i) {
		i++
	}
	return i
}

func (s *groupedStats) export(a Aggregation) (*pb.ClientGroupedStats, error) {
	msg := s.okDistribution.ToProto()
	okSummary, err := proto.Marshal(msg)
	if err != nil {
		return &pb.ClientGroupedStats{}, err
	}
	msg = s.errDistribution.ToProto()
	errSummary, err := proto.Marshal(msg)
	if err != nil {
		return &pb.ClientGroupedStats{}, err
	}
	return &pb.ClientGroupedStats{
		Service:              a.Service,
		Name:                 a.Name,
		Resource:             a.Resource,
		HTTPStatusCode:       a.StatusCode,
		Type:                 a.Type,
		Hits:                 round(s.hits),
		Errors:               round(s.errors),
		Duration:             round(s.duration),
		TopLevelHits:         round(s.topLevelHits),
		OkSummary:            okSummary,
		ErrorSummary:         errSummary,
		Synthetics:           a.Synthetics,
		SpanKind:             a.SpanKind,
		PeerTags:             s.peerTags,
		AdditionalMetricTags: s.additionalMetricTags,
		ServiceSource:        a.ServiceSource,
		IsTraceRoot:          a.IsTraceRoot,
		GRPCStatusCode:       a.GRPCStatusCode,
		HTTPMethod:           a.HTTPMethod,
		HTTPEndpoint:         a.HTTPEndpoint,
	}, nil
}

func newGroupedStats() *groupedStats {
	okSketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	if err != nil {
		log.Errorf("Error when creating ddsketch: %v", err)
	}
	errSketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	if err != nil {
		log.Errorf("Error when creating ddsketch: %v", err)
	}
	return &groupedStats{
		okDistribution:  okSketch,
		errDistribution: errSketch,
	}
}

// BucketCardinalityLimits holds per-field and whole-key cardinality limits for a RawBucket.
// A value of 0 disables the cap for that field. These are tracer-only controls;
// the Agent intentionally leaves them all at 0 (no-op).
type BucketCardinalityLimits struct {
	AdditionalTags int
	Resource       int
	HTTPEndpoint   int
	PeerTags       int
	Origin         int
	WholeKey       int
}

// SpanCollapseResult reports which cardinality collapses were applied to a span in HandleSpan.
type SpanCollapseResult struct {
	WholeKeyCollapsed      bool
	ResourceCollapsed      bool
	HTTPEndpointCollapsed  bool
	PeerTagsCollapsed      bool
	OriginCollapsed        bool
	AdditionalTagsCapBlock bool
}

// RawBucket is used to compute span data and aggregate it
// within a time-framed bucket. This should not be used outside
// the agent, use ClientStatsBucket for this.
type RawBucket struct {
	// This should really have no public fields. At all.

	start    uint64 // timestamp of start in our format
	duration uint64 // duration of a bucket in nanoseconds

	// this should really remain private as it's subject to refactoring
	data map[Aggregation]*groupedStats

	additionalMetricTagValueBlockSentinel string

	// per-field cardinality limits and trackers (nil map = limit disabled)
	additionalTagsCardinalityLimit int
	additionalTagsEntries          int
	resourceCardinalityLimit       int
	distinctResources              map[string]struct{}
	httpEndpointCardinalityLimit   int
	distinctHTTPEndpoints          map[string]struct{}
	peerTagsCardinalityLimit       int
	distinctPeerTagHashes          map[uint64]struct{}
	originCardinalityLimit         int
	distinctOrigins                map[string]struct{}

	// whole-key cardinality limit: caps total distinct BucketsAggregationKeys per bucket
	wholeKeyCardinalityLimit int
	distinctWholeKeys        map[BucketsAggregationKey]struct{}

	// warnedThisBucket is set the first time any collapse fires in this bucket
	// so that at most one debug log is emitted per flush window.
	warnedThisBucket bool

	containerTagsByID map[string][]string // a map from container ID to container tags
	processTagsByHash map[uint64]string   // a map from process hash to process tags
}

// NewRawBucket opens a new calculation bucket for time ts and initializes it properly
func NewRawBucket(ts, d uint64, limits BucketCardinalityLimits) *RawBucket {
	rb := &RawBucket{
		start:                                 ts,
		duration:                              d,
		data:                                  make(map[Aggregation]*groupedStats),
		additionalMetricTagValueBlockSentinel: blockedByTracerSentinel,
		additionalTagsCardinalityLimit:        limits.AdditionalTags,
		resourceCardinalityLimit:              limits.Resource,
		httpEndpointCardinalityLimit:          limits.HTTPEndpoint,
		peerTagsCardinalityLimit:              limits.PeerTags,
		originCardinalityLimit:                limits.Origin,
		wholeKeyCardinalityLimit:              limits.WholeKey,
		containerTagsByID:                     make(map[string][]string),
		processTagsByHash:                     make(map[uint64]string),
	}
	if limits.Resource > 0 {
		rb.distinctResources = make(map[string]struct{})
	}
	if limits.HTTPEndpoint > 0 {
		rb.distinctHTTPEndpoints = make(map[string]struct{})
	}
	if limits.PeerTags > 0 {
		rb.distinctPeerTagHashes = make(map[uint64]struct{})
	}
	if limits.Origin > 0 {
		rb.distinctOrigins = make(map[string]struct{})
	}
	if limits.WholeKey > 0 {
		rb.distinctWholeKeys = make(map[BucketsAggregationKey]struct{})
	}
	return rb
}

// Export transforms a RawBucket into a ClientStatsBucket, typically used
// before communicating data to the API, as RawBucket is the internal
// type while ClientStatsBucket is the public, shared one.
func (sb *RawBucket) Export() map[PayloadAggregationKey]*pb.ClientStatsBucket {
	m := make(map[PayloadAggregationKey]*pb.ClientStatsBucket)
	for k, v := range sb.data {
		b, err := v.export(k)
		if err != nil {
			log.Errorf("Dropping stats bucket due to encoding error: %v.", err)
			continue
		}
		key := PayloadAggregationKey{
			Hostname:        k.Hostname,
			Version:         k.Version,
			Env:             k.Env,
			ContainerID:     k.ContainerID,
			GitCommitSha:    k.GitCommitSha,
			ImageTag:        k.ImageTag,
			Lang:            k.Lang,
			ProcessTagsHash: k.ProcessTagsHash,
			BaseService:     k.BaseService,
		}
		s, ok := m[key]
		if !ok {
			s = &pb.ClientStatsBucket{
				Start:    sb.start,
				Duration: sb.duration,
			}
		}
		s.Stats = append(s.Stats, b)
		m[key] = s
	}
	return m
}

// HandleSpan adds the span to this bucket stats, aggregated with the finest grain matching given aggregators.
// It returns a SpanCollapseResult indicating which cardinality collapses were applied.
func (sb *RawBucket) HandleSpan(s *StatSpan, weight float64, origin string, aggKey PayloadAggregationKey) SpanCollapseResult {
	if aggKey.Env == "" {
		panic("env should never be empty")
	}
	var result SpanCollapseResult
	aggr := NewAggregationFromSpan(s, origin, aggKey)

	// --- Whole-key cardinality check (backstop) ---
	// Runs before per-field checks: if the full key would exceed the whole-key limit,
	// collapse every field to the sentinel at once.
	if sb.wholeKeyCardinalityLimit > 0 {
		if _, exists := sb.distinctWholeKeys[aggr.BucketsAggregationKey]; !exists {
			if len(sb.distinctWholeKeys) >= sb.wholeKeyCardinalityLimit {
				if !sb.warnedThisBucket {
					log.Debugf("stats cardinality whole-key limit (%d) reached for this bucket; collapsing span to sentinel", sb.wholeKeyCardinalityLimit)
					sb.warnedThisBucket = true
				}
				sentinel := sb.getAdditionalMetricTagValueBlockSentinel()
				s.resource = sentinel
				s.name = sentinel
				s.typ = sentinel
				s.spanKind = sentinel
				s.httpMethod = sentinel
				s.httpEndpoint = sentinel
				s.service = sentinel
				s.serviceSource = sentinel
				s.matchingPeerTags = []string{sentinel}
				s.matchingAdditionalMetricTags = []string{sentinel}
				s.statusCode = 0
				s.grpcStatusCode = ""
				// isTraceRoot becomes NOT_SET (0) via parentID trick: use a special collapsed aggregation
				aggr = wholeKeyCollapsedAggregation(aggKey, sentinel)
				result.WholeKeyCollapsed = true
				sb.add(s, weight, aggr)
				return result
			}
			sb.distinctWholeKeys[aggr.BucketsAggregationKey] = struct{}{}
		}
	}

	// --- Per-field cardinality checks ---
	// Each field is checked independently; collapsed fields use the sentinel value.
	// After any per-field collapse, aggr is recomputed.
	recomputeAggr := false
	sentinel := sb.getAdditionalMetricTagValueBlockSentinel()

	if sb.resourceCardinalityLimit > 0 && s.resource != "" {
		if _, exists := sb.distinctResources[s.resource]; !exists {
			if len(sb.distinctResources) >= sb.resourceCardinalityLimit {
				if !sb.warnedThisBucket {
					log.Debugf("stats cardinality resource limit (%d) reached for this bucket; collapsing resource to sentinel", sb.resourceCardinalityLimit)
					sb.warnedThisBucket = true
				}
				s.resource = sentinel
				result.ResourceCollapsed = true
				recomputeAggr = true
			} else {
				sb.distinctResources[s.resource] = struct{}{}
			}
		}
	}

	if sb.httpEndpointCardinalityLimit > 0 && s.httpEndpoint != "" {
		if _, exists := sb.distinctHTTPEndpoints[s.httpEndpoint]; !exists {
			if len(sb.distinctHTTPEndpoints) >= sb.httpEndpointCardinalityLimit {
				if !sb.warnedThisBucket {
					log.Debugf("stats cardinality http_endpoint limit (%d) reached for this bucket; collapsing http_endpoint to sentinel", sb.httpEndpointCardinalityLimit)
					sb.warnedThisBucket = true
				}
				s.httpEndpoint = sentinel
				result.HTTPEndpointCollapsed = true
				recomputeAggr = true
			} else {
				sb.distinctHTTPEndpoints[s.httpEndpoint] = struct{}{}
			}
		}
	}

	if sb.peerTagsCardinalityLimit > 0 && len(s.matchingPeerTags) > 0 {
		h := tagsFnvHash(s.matchingPeerTags)
		if _, exists := sb.distinctPeerTagHashes[h]; !exists {
			if len(sb.distinctPeerTagHashes) >= sb.peerTagsCardinalityLimit {
				if !sb.warnedThisBucket {
					log.Debugf("stats cardinality peer_tags limit (%d) reached for this bucket; collapsing peer_tags to sentinel", sb.peerTagsCardinalityLimit)
					sb.warnedThisBucket = true
				}
				s.matchingPeerTags = []string{sentinel}
				result.PeerTagsCollapsed = true
				recomputeAggr = true
			} else {
				sb.distinctPeerTagHashes[h] = struct{}{}
			}
		}
	}

	if sb.originCardinalityLimit > 0 && origin != "" {
		if _, exists := sb.distinctOrigins[origin]; !exists {
			if len(sb.distinctOrigins) >= sb.originCardinalityLimit {
				if !sb.warnedThisBucket {
					log.Debugf("stats cardinality origin limit (%d) reached for this bucket; collapsing origin to empty", sb.originCardinalityLimit)
					sb.warnedThisBucket = true
				}
				// Origin is not a field on StatSpan — it controls the Synthetics flag in the aggregation.
				// We collapse by clearing the origin so Synthetics=false in the overflow aggregation.
				origin = ""
				result.OriginCollapsed = true
				recomputeAggr = true
			} else {
				sb.distinctOrigins[origin] = struct{}{}
			}
		}
	}

	if recomputeAggr {
		aggr = NewAggregationFromSpan(s, origin, aggKey)
	}

	// --- additional_metric_tags per-bucket cardinality cap (pre-existing logic) ---
	if len(s.matchingAdditionalMetricTags) > 0 && sb.additionalTagsCardinalityLimit > 0 {
		// Only new tag-bearing aggregations are counted, before they are added below.
		if _, exists := sb.data[aggr]; !exists {
			if sb.additionalTagsEntries >= sb.additionalTagsCardinalityLimit {
				if !sb.warnedThisBucket {
					log.Debugf("stats cardinality additional_metric_tags limit (%d) reached for this bucket; masking %d tag value(s)", sb.additionalTagsCardinalityLimit, len(s.matchingAdditionalMetricTags))
					sb.warnedThisBucket = true
				}
				// Cap reached: collapse this span's tag values onto the shared masked
				// aggregation. The masked entry is deliberately NOT counted toward the
				// limit — all over-cap spans fold into it, so it stays a single overflow
				// bucket rather than consuming a slot.
				s.matchingAdditionalMetricTags = maskAdditionalMetricTagValues(s.matchingAdditionalMetricTags, sb.getAdditionalMetricTagValueBlockSentinel())
				aggr = NewAggregationFromSpan(s, origin, aggKey)
				result.AdditionalTagsCapBlock = true
			} else {
				sb.additionalTagsEntries++
			}
		}
	}

	sb.add(s, weight, aggr)
	return result
}

// wholeKeyCollapsedAggregation returns an Aggregation where every operational field
// is set to the sentinel value, preserving only the deployment-level (payload) key.
// is_trace_root is NOT_SET (0) and synthetics is false as required by the RFC.
func wholeKeyCollapsedAggregation(aggKey PayloadAggregationKey, sentinel string) Aggregation {
	sentinelHash := tagsFnvHash([]string{sentinel})
	return Aggregation{
		PayloadAggregationKey: aggKey,
		BucketsAggregationKey: BucketsAggregationKey{
			Service:                  sentinel,
			Name:                     sentinel,
			Resource:                 sentinel,
			Type:                     sentinel,
			SpanKind:                 sentinel,
			StatusCode:               0,
			Synthetics:               false,
			PeerTagsHash:             sentinelHash,
			AdditionalMetricTagsHash: sentinelHash,
			ServiceSource:            sentinel,
			IsTraceRoot:              0, // NOT_SET
			GRPCStatusCode:           "",
			HTTPMethod:               sentinel,
			HTTPEndpoint:             sentinel,
		},
	}
}

func (sb *RawBucket) getAdditionalMetricTagValueBlockSentinel() string {
	if sb.additionalMetricTagValueBlockSentinel == "" {
		return blockedByTracerSentinel
	}
	return sb.additionalMetricTagValueBlockSentinel
}

func (sb *RawBucket) add(s *StatSpan, weight float64, aggr Aggregation) {
	var gs *groupedStats
	var ok bool

	if gs, ok = sb.data[aggr]; !ok {
		gs = newGroupedStats()
		gs.peerTags = s.matchingPeerTags
		gs.additionalMetricTags = s.matchingAdditionalMetricTags
		sb.data[aggr] = gs
	}
	if s.isTopLevel {
		gs.topLevelHits += weight
	}
	gs.hits += weight
	if s.error != 0 {
		gs.errors += weight
	}
	gs.duration += float64(s.duration) * weight
	// alter resolution of duration distro
	trundur := nsTimestampToFloat(s.duration)
	if s.error != 0 {
		if err := gs.errDistribution.Add(trundur); err != nil {
			log.Debugf("Error adding error distribution stats: %v", err)
		}
	} else {
		if err := gs.okDistribution.Add(trundur); err != nil {
			log.Debugf("Error adding distribution stats: %v", err)
		}
	}
}

// nsTimestampToFloat converts a nanosec timestamp into a float nanosecond timestamp truncated to a fixed precision
func nsTimestampToFloat(ns int64) float64 {
	b := math.Float64bits(float64(ns))
	// IEEE-754
	// the mask include 1 bit sign 11 bits exponent (0xfff)
	// then we filter the mantissa to 10bits (0xff8) (9 bits as it has implicit value of 1)
	// 10 bits precision (any value will be +/- 1/1024)
	// https://en.wikipedia.org/wiki/Double-precision_floating-point_format
	b &= 0xfffff80000000000
	return math.Float64frombits(b)
}
