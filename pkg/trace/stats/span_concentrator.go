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
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// topLevelKey is a special metric, it's 1 if the span is top-level, 0 if not.
const topLevelKey = "_top_level"

// measuredKey is a special metric flag that marks a span for trace metrics calculation.
const measuredKey = "_dd.measured"

// partialVersionKey is a metric carrying the snapshot seq number in the case the span is a partial snapshot
const partialVersionKey = "_dd.partial_version"

// SpanConcentratorConfig exposes configuration options for a SpanConcentrator
type SpanConcentratorConfig struct {
	// ComputeStatsBySpanKind enables/disables the computing of stats based on a span's `span.kind` field
	ComputeStatsBySpanKind bool
	// BucketInterval the size of our pre-aggregation per bucket
	BucketInterval int64
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

	spanKind         string
	statusCode       uint32
	isTopLevel       bool
	matchingPeerTags []string
}

func matchingPeerTags(meta map[string]string, peerTagKeys []string) []string {
	if len(peerTagKeys) == 0 {
		return nil
	}
	var pt []string
	for _, t := range peerTagKeysToAggregateForSpan(meta[tagSpanKind], meta[tagBaseService], peerTagKeys) {
		if v, ok := meta[t]; ok && v != "" {
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
		return []string{tagBaseService}
	}
	if spanKind == "client" || spanKind == "producer" || spanKind == "consumer" {
		return peerTagKeys
	}
	return nil
}

// SpanConcentrator produces time bucketed statistics from a stream of raw spans.
type SpanConcentrator struct {
	computeStatsBySpanKind bool
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
	sc := &SpanConcentrator{
		computeStatsBySpanKind: false,
		bsize:                  cfg.BucketInterval,
		oldestTs:               alignTs(now.UnixNano(), cfg.BucketInterval),
		bufferLen:              defaultBufferLen,
		mu:                     sync.Mutex{},
		buckets:                make(map[int64]*RawBucket),
	}
	return sc
}

// NewStatSpanFromPB is a helper version of NewStatSpan that builds a StatSpan from a pb.Span.
func (sc *SpanConcentrator) NewStatSpanFromPB(s *pb.Span, peerTags []string) (statSpan *StatSpan, ok bool) {
	return sc.NewStatSpan(s.Service, s.Resource, s.Name, s.Type, s.ParentID, s.Start, s.Duration, s.Error, s.Meta, s.Metrics, peerTags)
}

// NewStatSpan builds a StatSpan from the required fields for stats calculation
// peerTags is the configured list of peer tags to look for
// returns (nil,false) if the provided fields indicate a span should not have stats calculated
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
	if meta == nil {
		meta = make(map[string]string)
	}
	if metrics == nil {
		metrics = make(map[string]float64)
	}
	partialVersion, hasPartialVersion := metrics[partialVersionKey]
	eligibleSpanKind := sc.computeStatsBySpanKind && computeStatsForSpanKind(meta["span.kind"])
	if !(metrics[topLevelKey] == 1 || metrics[measuredKey] == 1 || eligibleSpanKind) {
		return nil, false
	}
	if hasPartialVersion && partialVersion >= 0 {
		return nil, false
	}
	return &StatSpan{
		service:          service,
		resource:         resource,
		name:             name,
		typ:              typ,
		error:            error,
		parentID:         parentID,
		start:            start,
		duration:         duration,
		spanKind:         meta[tagSpanKind],
		statusCode:       getStatusCode(meta, metrics),
		isTopLevel:       metrics[topLevelKey] == 1,
		matchingPeerTags: matchingPeerTags(meta, peerTags),
	}, true
}

// computeStatsForSpanKind returns true if the span.kind value makes the span eligible for stats computation.
func computeStatsForSpanKind(kind string) bool {
	k := strings.ToLower(kind)
	switch k {
	case "server", "consumer", "client", "producer":
		return true
	default:
		return false
	}
}

func (sc *SpanConcentrator) addSpan(s *StatSpan, aggKey PayloadAggregationKey, containerID string, containerTags []string, origin string, weight float64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	end := s.start + s.duration
	btime := end - end%sc.bsize

	// If too far in the past, count in the oldest-allowed time bucket instead.
	if btime < sc.oldestTs {
		btime = sc.oldestTs
	}

	b, ok := sc.buckets[btime]
	if !ok {
		b = NewRawBucket(uint64(btime), uint64(sc.bsize))
		if containerID != "" && len(containerTags) > 0 {
			b.containerTagsByID[containerID] = containerTags
		}
		sc.buckets[btime] = b
	}
	b.HandleSpan(s, weight, origin, aggKey)
}

// AddSpan to the SpanConcentrator, appending the new data to the appropriate internal bucket.
func (sc *SpanConcentrator) AddSpan(s *StatSpan, aggKey PayloadAggregationKey, containerID string, containerTags []string, origin string) {
	sc.addSpan(s, aggKey, containerID, containerTags, origin, 1)
}

// Flush deletes and returns complete ClientStatsPayloads.
// The force boolean guarantees flushing all buckets if set to true.
func (sc *SpanConcentrator) Flush(now int64, force bool) []*pb.ClientStatsPayload {
	m := make(map[PayloadAggregationKey][]*pb.ClientStatsBucket)
	containerTagsByID := make(map[string][]string)

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
			Env:          k.Env,
			Hostname:     k.Hostname,
			ContainerID:  k.ContainerID,
			Version:      k.Version,
			GitCommitSha: k.GitCommitSha,
			ImageTag:     k.ImageTag,
			Stats:        s,
			Tags:         containerTagsByID[k.ContainerID],
		}
		sb = append(sb, p)
	}
	return sb
}
