// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/version"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/DataDog/sketches-go/ddsketch/store"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"

	"github.com/DataDog/datadog-go/v5/statsd"

	"google.golang.org/protobuf/proto"
)

const (
	bucketDuration       = 2 * time.Second
	clientBucketDuration = 10 * time.Second
	oldestBucketStart    = 20 * time.Second
)

var (
	ddsketchMapping, _ = mapping.NewLogarithmicMapping(relativeAccuracy)
)

// ClientStatsAggregator aggregates client stats payloads on buckets of bucketDuration
// If a single payload is received on a bucket, this Aggregator is a passthrough.
// If two or more payloads collide, their counts will be aggregated into one bucket.
// Multiple payloads will be sent:
// - Original payloads with their distributions will be sent with counts zeroed.
// - A single payload with the bucket aggregated counts will be sent.
// This and the aggregator timestamp alignment ensure that all counts will have at most one point per second per agent for a specific granularity.
// While distributions are not tied to the agent.
type ClientStatsAggregator struct {
	In      chan *pb.ClientStatsPayload
	writer  Writer
	buckets map[int64]*bucket // buckets used to aggregate client stats
	conf    *config.AgentConfig

	flushTicker   *time.Ticker
	oldestTs      time.Time
	agentEnv      string
	agentHostname string
	agentVersion  string

	exit chan struct{}
	done chan struct{}

	exitWG sync.WaitGroup
	mu     sync.Mutex

	statsd statsd.ClientInterface
}

// NewClientStatsAggregator initializes a new aggregator ready to be started
func NewClientStatsAggregator(conf *config.AgentConfig, writer Writer, statsd statsd.ClientInterface) *ClientStatsAggregator {
	flushInterval := conf.ClientStatsFlushInterval
	if flushInterval == 0 { // default to 2s if not set to ensure users of this outside the agent aren't broken by this change
		flushInterval = 2 // bucketDuration
	}
	c := &ClientStatsAggregator{
		flushTicker:   time.NewTicker(flushInterval),
		In:            make(chan *pb.ClientStatsPayload, 10),
		buckets:       make(map[int64]*bucket, 20),
		conf:          conf,
		writer:        writer,
		agentEnv:      conf.DefaultEnv,
		agentHostname: conf.Hostname,
		agentVersion:  conf.AgentVersion,
		oldestTs:      alignAggTs(time.Now().Add(bucketDuration - oldestBucketStart)),
		exit:          make(chan struct{}),
		done:          make(chan struct{}),
		statsd:        statsd,
	}
	return c
}

// Start starts the aggregator.
func (a *ClientStatsAggregator) Start() {
	// 2 goroutines: aggregation and flushing
	a.exitWG.Add(2)

	// aggregation goroutine
	go func() {
		defer watchdog.LogOnPanic(a.statsd)
		defer a.exitWG.Done()
		for {
			select {
			case input := <-a.In:
				a.add(time.Now(), input)
			case <-a.exit:
				return
			}
		}
	}()

	// flushing goroutine
	go func() {
		defer watchdog.LogOnPanic(a.statsd)
		defer a.exitWG.Done()
		for {
			select {
			case t := <-a.flushTicker.C:
				a.flushOnTime(t)
			case <-a.exit:
				a.flushAll()
				return
			}
		}
	}()
}

// Stop stops the aggregator. Calling Stop twice will panic.
func (a *ClientStatsAggregator) Stop() {
	close(a.exit)
	a.flushTicker.Stop()
	a.exitWG.Wait()
}

// flushOnTime flushes all buckets up to flushTs, except the last one.
func (a *ClientStatsAggregator) flushOnTime(now time.Time) {
	a.flush(now, false)
}

// flushAll flushes all buckets, typically called on agent shutdown.
func (a *ClientStatsAggregator) flushAll() {
	a.flush(time.Now(), true)
}

func (a *ClientStatsAggregator) flush(now time.Time, force bool) {
	flushTs := alignAggTs(now.Add(bucketDuration - oldestBucketStart))

	a.mu.Lock()
	defer a.mu.Unlock()

	for ts, b := range a.buckets {
		if !force && !flushTs.After(b.ts) {
			continue
		}
		log.Debugf("css aggregator: flushing bucket %d", ts)
		a.flushPayloads(b.aggregationToPayloads())
		delete(a.buckets, ts)
	}
	a.oldestTs = flushTs
}

// getAggregationBucketTime returns unix time at which we aggregate the bucket.
// We timeshift payloads older than a.oldestTs to a.oldestTs.
// Payloads in the future are timeshifted to the latest bucket.
func (a *ClientStatsAggregator) getAggregationBucketTime(now, bs time.Time) time.Time {
	if bs.Before(a.oldestTs) {
		return a.oldestTs
	}
	if bs.After(now) {
		return alignAggTs(now)
	}
	return alignAggTs(bs)
}

// add takes a new ClientStatsPayload and aggregates its stats in the internal buckets.
func (a *ClientStatsAggregator) add(now time.Time, p *pb.ClientStatsPayload) {
	// populate container tags data on the payload
	a.setVersionDataFromContainerTags(p)
	p.ProcessTagsHash = processTagsHash(p.ProcessTags)
	// compute the PayloadAggregationKey, common for all buckets within the payload
	payloadAggKey := newPayloadAggregationKey(p.Env, p.Hostname, p.Version, p.ContainerID, p.GitCommitSha, p.ImageTag, p.Lang, p.ProcessTagsHash)

	// acquire lock over shared data
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, clientBucket := range p.Stats {
		clientBucketStart := time.Unix(0, int64(clientBucket.Start))
		ts := a.getAggregationBucketTime(now, clientBucketStart)
		b, ok := a.buckets[ts.Unix()]
		if !ok {
			b = &bucket{
				ts:          ts,
				agg:         make(map[PayloadAggregationKey]map[BucketsAggregationKey]*aggregatedStats),
				processTags: make(map[uint64]string),
			}
			a.buckets[ts.Unix()] = b
		}
		b.processTags[p.ProcessTagsHash] = p.ProcessTags
		b.aggregateStatsBucket(clientBucket, payloadAggKey)
	}
}

func (a *ClientStatsAggregator) flushPayloads(p []*pb.ClientStatsPayload) {
	if len(p) == 0 {
		return
	}

	a.writer.Write(&pb.StatsPayload{
		Stats:          p,
		AgentEnv:       a.agentEnv,
		AgentHostname:  a.agentHostname,
		AgentVersion:   a.agentVersion,
		ClientComputed: true,
	})
}

func (a *ClientStatsAggregator) setVersionDataFromContainerTags(p *pb.ClientStatsPayload) {
	// No need to go any further if we already have the information in the payload.
	if p.ImageTag != "" && p.GitCommitSha != "" {
		return
	}
	if p.ContainerID != "" {
		cTags, err := a.conf.ContainerTags(p.ContainerID)
		if err != nil {
			log.Error("Client stats aggregator is unable to resolve container ID (%s) to container tags: %v", p.ContainerID, err)
		} else {
			gitCommitSha, imageTag := version.GetVersionDataFromContainerTags(cTags)
			// Only override if the payload's original values were empty strings.
			if p.ImageTag == "" {
				p.ImageTag = imageTag
			}
			if p.GitCommitSha == "" {
				p.GitCommitSha = gitCommitSha
			}
		}
	}
}

// alignAggTs aligns time to the aggregator timestamps.
// Timestamps from the aggregator are never aligned  with concentrator timestamps.
// This ensures that all counts sent by a same agent host are never on the same second.
// aggregator timestamps:   2ks+1s (1s, 3s, 5s, 7s, 9s, 11s)
// concentrator timestamps: 10ks   (0s, 10s, 20s ..)
func alignAggTs(t time.Time) time.Time {
	return t.Truncate(bucketDuration).Add(time.Second)
}

type bucket struct {
	// ts is the timestamp attached to the payload
	ts time.Time
	// agg contains the aggregated Hits/Errors/Duration counts
	agg         map[PayloadAggregationKey]map[BucketsAggregationKey]*aggregatedStats
	processTags map[uint64]string
}

// aggregateStatsBucket takes a ClientStatsBucket and a PayloadAggregationKey, and aggregates all counts
// and distributions from the ClientGroupedStats inside the bucket.
func (b *bucket) aggregateStatsBucket(sb *pb.ClientStatsBucket, payloadAggKey PayloadAggregationKey) {
	payloadAgg, ok := b.agg[payloadAggKey]
	if !ok {
		payloadAgg = make(map[BucketsAggregationKey]*aggregatedStats, len(sb.Stats))
		b.agg[payloadAggKey] = payloadAgg
	}
	for _, gs := range sb.Stats {
		if gs == nil {
			continue
		}
		aggKey := newBucketAggregationKey(gs)
		agg, ok := payloadAgg[aggKey]
		if !ok {
			agg = &aggregatedStats{
				hits:               gs.Hits,
				topLevelHits:       gs.TopLevelHits,
				errors:             gs.Errors,
				duration:           gs.Duration,
				peerTags:           gs.PeerTags,
				okDistributionRaw:  gs.OkSummary,    // store encoded version only
				errDistributionRaw: gs.ErrorSummary, // store encoded version only
			}
			payloadAgg[aggKey] = agg
			continue
		}

		// aggregate counts
		agg.hits += gs.Hits
		agg.topLevelHits += gs.TopLevelHits
		agg.errors += gs.Errors
		agg.duration += gs.Duration

		// Decode, if needed, the raw ddsketches from the first payload that reached the bucket
		if agg.okDistributionRaw != nil {
			sketch, err := decodeSketch(agg.okDistributionRaw)
			if err != nil {
				log.Error("Unable to decode OK distribution ddsketch: %v", err)
			} else {
				agg.okDistribution = normalizeSketch(sketch)
			}
			agg.okDistributionRaw = nil
		}
		if agg.errDistributionRaw != nil {
			sketch, err := decodeSketch(agg.errDistributionRaw)
			if err != nil {
				log.Error("Unable to decode Error distribution ddsketch: %v", err)
			} else {
				agg.errDistribution = normalizeSketch(sketch)
			}
			agg.errDistributionRaw = nil
		}

		// aggregate distributions
		if sketch, err := mergeSketch(agg.okDistribution, gs.OkSummary); err == nil {
			agg.okDistribution = sketch
		} else {
			log.Error("Unable to merge OK distribution ddsketch: %v", err)
		}

		if sketch, err := mergeSketch(agg.errDistribution, gs.ErrorSummary); err == nil {
			agg.errDistribution = sketch
		} else {
			log.Error("Unable to merge Error distribution ddsketch: %v", err)
		}
	}
}

// aggregationToPayloads converts the contents of the bucket into ClientStatsPayloads
func (b *bucket) aggregationToPayloads() []*pb.ClientStatsPayload {
	res := make([]*pb.ClientStatsPayload, 0, len(b.agg))
	for payloadKey, aggrStats := range b.agg {
		groupedStats := make([]*pb.ClientGroupedStats, 0, len(aggrStats))
		for aggrKey, stats := range aggrStats {
			gs, err := exporGroupedStats(aggrKey, stats)
			if err != nil {
				log.Errorf("Dropping stats bucket due to encoding error: %v.", err)
				continue
			}
			groupedStats = append(groupedStats, gs)
		}
		clientBuckets := []*pb.ClientStatsBucket{
			{
				Start:    uint64(b.ts.UnixNano()),
				Duration: uint64(clientBucketDuration.Nanoseconds()),
				Stats:    groupedStats,
			}}
		res = append(res, &pb.ClientStatsPayload{
			Hostname:        payloadKey.Hostname,
			Env:             payloadKey.Env,
			Version:         payloadKey.Version,
			ImageTag:        payloadKey.ImageTag,
			Lang:            payloadKey.Lang,
			GitCommitSha:    payloadKey.GitCommitSha,
			ContainerID:     payloadKey.ContainerID,
			Stats:           clientBuckets,
			ProcessTagsHash: payloadKey.ProcessTagsHash,
			ProcessTags:     b.processTags[payloadKey.ProcessTagsHash],
		})
	}
	return res
}

func exporGroupedStats(aggrKey BucketsAggregationKey, stats *aggregatedStats) (*pb.ClientGroupedStats, error) {
	// if the raw sketches are still present (only one payload received), we use them directly.
	// Otherwise the aggregated DDSketches are serialized.
	okSummary := stats.okDistributionRaw
	errSummary := stats.errDistributionRaw

	var err error
	if stats.okDistribution != nil {
		msg := stats.okDistribution.ToProto()
		okSummary, err = proto.Marshal(msg)
		if err != nil {
			return &pb.ClientGroupedStats{}, err
		}
	}
	if stats.errDistribution != nil {
		msg := stats.errDistribution.ToProto()
		errSummary, err = proto.Marshal(msg)
		if err != nil {
			return &pb.ClientGroupedStats{}, err
		}
	}
	return &pb.ClientGroupedStats{
		Service:        aggrKey.Service,
		Name:           aggrKey.Name,
		SpanKind:       aggrKey.SpanKind,
		Resource:       aggrKey.Resource,
		HTTPStatusCode: aggrKey.StatusCode,
		Type:           aggrKey.Type,
		Synthetics:     aggrKey.Synthetics,
		IsTraceRoot:    aggrKey.IsTraceRoot,
		GRPCStatusCode: aggrKey.GRPCStatusCode,
		HTTPMethod:     aggrKey.HTTPMethod,
		HTTPEndpoint:   aggrKey.HTTPEndpoint,
		PeerTags:       stats.peerTags,
		TopLevelHits:   stats.topLevelHits,
		Hits:           stats.hits,
		Errors:         stats.errors,
		Duration:       stats.duration,
		OkSummary:      okSummary,
		ErrorSummary:   errSummary,
	}, nil
}

func newPayloadAggregationKey(env, hostname, version, cid, gitCommitSha, imageTag, lang string, processTagsHash uint64) PayloadAggregationKey {
	return PayloadAggregationKey{
		Env:             env,
		Hostname:        hostname,
		Version:         version,
		ContainerID:     cid,
		GitCommitSha:    gitCommitSha,
		ImageTag:        imageTag,
		Lang:            lang,
		ProcessTagsHash: processTagsHash,
	}
}

func newBucketAggregationKey(b *pb.ClientGroupedStats) BucketsAggregationKey {
	k := BucketsAggregationKey{
		Service:        b.Service,
		Name:           b.Name,
		SpanKind:       b.SpanKind,
		Resource:       b.Resource,
		Type:           b.Type,
		Synthetics:     b.Synthetics,
		StatusCode:     b.HTTPStatusCode,
		GRPCStatusCode: b.GRPCStatusCode,
		IsTraceRoot:    b.IsTraceRoot,
		HTTPMethod:     b.HTTPMethod,
		HTTPEndpoint:   b.HTTPEndpoint,
	}
	if tags := b.GetPeerTags(); len(tags) > 0 {
		k.PeerTagsHash = tagsFnvHash(tags)
	}
	return k
}

// aggregatedStats holds aggregated counts and distributions
type aggregatedStats struct {
	// aggregated counts
	hits, topLevelHits, errors, duration uint64
	peerTags                             []string

	// aggregated DDSketches
	okDistribution, errDistribution *ddsketch.DDSketch

	// raw (encoded) DDSketches. Only present if a single payload is received on the active bucket,
	// allowing the bucket to not decode the sketch. If a second payload matches the bucket,
	// sketches will be decoded and stored in the okDistribution and errDistribution fields.
	okDistributionRaw, errDistributionRaw []byte
}

// mergeSketch take an existing DDSketch, and merges a second one, decoding its contents
func mergeSketch(s1 *ddsketch.DDSketch, raw []byte) (*ddsketch.DDSketch, error) {
	if raw == nil {
		return s1, nil
	}

	s2, err := decodeSketch(raw)
	if err != nil {
		return s1, err
	}
	s2 = normalizeSketch(s2)

	if s1 == nil {
		return s2, nil
	}

	if err = s1.MergeWith(s2); err != nil {
		return nil, err
	}
	return s1, nil
}

func normalizeSketch(s *ddsketch.DDSketch) *ddsketch.DDSketch {
	if s.IndexMapping.Equals(ddsketchMapping) {
		// already normalized
		return s
	}

	return s.ChangeMapping(ddsketchMapping, store.NewCollapsingLowestDenseStore(maxNumBins), store.NewCollapsingLowestDenseStore(maxNumBins), 1)
}

func decodeSketch(data []byte) (*ddsketch.DDSketch, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var sketch sketchpb.DDSketch
	err := proto.Unmarshal(data, &sketch)
	if err != nil {
		return nil, err
	}

	return ddsketch.FromProto(&sketch)
}
