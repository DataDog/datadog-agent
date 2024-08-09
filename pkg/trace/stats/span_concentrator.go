// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"sync"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// SpanConcentratorConfig exposes configuration options for a SpanConcentrator
type SpanConcentratorConfig struct {
	// ComputeStatsBySpanKind enables/disables the computing of stats based on a span's `span.kind` field
	ComputeStatsBySpanKind bool
	// BucketInterval the size of our pre-aggregation per bucket
	BucketInterval int64
	// PeerTags additional tags to use for peer entity stats aggregation, nil if disabled
	PeerTags []string
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
	bufferLen   int
	peerTagKeys []string // keys for supplementary tags that describe peer.service entities, nil if disabled

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
		peerTagKeys:            cfg.PeerTags,
	}
	return sc
}

func (sc *SpanConcentrator) addSpan(s *pb.Span, aggKey PayloadAggregationKey, containerID string, containerTags []string, origin string, weight float64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	isTop := traceutil.HasTopLevel(s)
	eligibleSpanKind := sc.computeStatsBySpanKind && computeStatsForSpanKind(s)
	if !(isTop || traceutil.IsMeasured(s) || eligibleSpanKind) {
		return
	}
	if traceutil.IsPartialSnapshot(s) {
		return
	}
	end := s.Start + s.Duration
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
	b.HandleSpan(s, weight, isTop, origin, aggKey, sc.peerTagKeys)
}

// AddSpan to the SpanConcentrator, appending the new data to the appropriate internal bucket.
func (sc *SpanConcentrator) AddSpan(s *pb.Span, aggKey PayloadAggregationKey, containerID string, containerTags []string, origin string) {
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
