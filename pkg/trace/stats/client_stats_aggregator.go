package stats

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

const (
	aggBucketSize           = uint64(1e9)
	bucketDuration          = uint64(1e10) // 10s
	oldestBucketStartSecond = uint64(20)   // 20s
	distributionsPayload    = "distributions"
	aggregatedCountPayload  = "counts"
)

// ClientStatsAggregator aggregates client stats payloads on 1s buckets.
// If a single payload is received on a bucket, this Aggregator is a passthrough.
// If two ore more payloads collide, all payloads will be sent with hits and errors
// set to 0. An aggregation will be started for Hits/Errors/Duration counts.
// The Aggregation will be sent through a new stat payload with no Histograms.
type ClientStatsAggregator struct {
	In      chan pb.ClientStatsPayload
	out     chan pb.StatsPayload
	buckets map[uint64]*bucket // bucketts used to aggregate client stats

	flushTicker   *time.Ticker
	oldestTs      uint64
	agentEnv      string
	agentHostname string

	exit chan struct{}
	done chan struct{}
}

// NewClientStatsAggregator initializes a new aggregator ready to be started
func NewClientStatsAggregator(conf *config.AgentConfig, out chan pb.StatsPayload) *ClientStatsAggregator {
	oldestTSUnix := uint64(time.Now().Add(-10 * time.Second).Unix())
	return &ClientStatsAggregator{
		flushTicker:   time.NewTicker(time.Second),
		In:            make(chan pb.ClientStatsPayload, 10),
		buckets:       make(map[uint64]*bucket, 20),
		out:           out,
		agentEnv:      conf.DefaultEnv,
		agentHostname: conf.Hostname,
		oldestTs:      oldestTSUnix,
	}
}

// Start starts the aggregator.
func (a *ClientStatsAggregator) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		a.run()
	}()
}

// Stop stops the aggregator.
func (a *ClientStatsAggregator) Stop() {
	close(a.exit)
	<-a.done
}

func (a *ClientStatsAggregator) run() {
	for {
		select {
		case t := <-a.flushTicker.C:
			a.flushOnTime(t)
		case input := <-a.In:
			a.add(time.Now(), input)
		case <-a.exit:
			a.flushAll()
			close(a.done)
		}
	}
}

func (a *ClientStatsAggregator) flushOnTime(t time.Time) {
	flushTs := uint64(t.Unix()) - oldestBucketStartSecond
	for ts := a.oldestTs; ts < flushTs; ts += 1 {
		if b, ok := a.buckets[ts]; ok {
			for _, p := range b.flush() {
				a.flush(p)
			}
			delete(a.buckets, ts)
		}
	}
	a.oldestTs = flushTs
}

func (a *ClientStatsAggregator) flushAll() {
	for _, b := range a.buckets {
		for _, p := range b.flush() {
			a.flush(p)
		}
	}
}

// getAggregationBucketTime returns unix time at which we aggregate the bucket.
// We timeshift payloads older than a.oldestTs to a.oldestTs.
// Payloads in the future are timeshifted to time.Now().Unix()
func (a *ClientStatsAggregator) getAggregationBucketTime(now time.Time, bucketStart uint64) (uint64, bool) {
	tsUnix := bucketStart / aggBucketSize
	if tsUnix < a.oldestTs {
		return a.oldestTs, true
	}
	nowUnix := uint64(now.Unix())
	if tsUnix > nowUnix {
		return nowUnix, true
	}
	return bucketStart / aggBucketSize, false
}

func (a *ClientStatsAggregator) add(now time.Time, p pb.ClientStatsPayload) {
	clientStats := p.Stats
	for _, clientBucket := range clientStats {
		tsUnix, timeShifted := a.getAggregationBucketTime(now, clientBucket.Start)
		if timeShifted {
			newStart := tsUnix * 1e9
			clientBucket.AgentTimeShift = int64(tsUnix) - int64(clientBucket.Start)
			clientBucket.Start = newStart
		}
		b, ok := a.buckets[tsUnix]
		if !ok {
			b = &bucket{tsUnix: tsUnix}
			a.buckets[tsUnix] = b
		}
		p.Stats = []pb.ClientStatsBucket{clientBucket}
		for _, p := range b.add(p) {
			a.flush(p)
		}
	}
}

func (a *ClientStatsAggregator) flush(p pb.ClientStatsPayload) {
	a.out <- pb.StatsPayload{
		Stats:          []pb.ClientStatsPayload{p},
		AgentEnv:       a.agentEnv,
		AgentHostname:  a.agentHostname,
		AgentVersion:   info.Version,
		ClientComputed: true,
	}
}

type bucket struct {
	firstPayload pb.ClientStatsPayload
	tsUnix       uint64
	numPayloads  int
	aggregator   map[payloadAggregationKey]map[bucketAggregationKey]*aggregatedCounts
}

func (b *bucket) add(p pb.ClientStatsPayload) []pb.ClientStatsPayload {
	b.numPayloads++
	if b.numPayloads == 1 {
		b.firstPayload = p
		return nil
	}
	if b.numPayloads == 2 {
		firstPayload := b.firstPayload
		b.firstPayload = pb.ClientStatsPayload{}
		b.aggregator = make(map[payloadAggregationKey]map[bucketAggregationKey]*aggregatedCounts, 2)
		b.aggregateHitsErrors(firstPayload)
		b.aggregateHitsErrors(p)
		firstPayload.AgentAggregation = distributionsPayload
		p.AgentAggregation = distributionsPayload
		return []pb.ClientStatsPayload{trimHitsErrors(firstPayload), trimHitsErrors(p)}
	}
	b.aggregateHitsErrors(p)
	p.AgentAggregation = distributionsPayload
	return []pb.ClientStatsPayload{trimHitsErrors(p)}
}

func (b *bucket) aggregateHitsErrors(p pb.ClientStatsPayload) {
	payloadAggKey := newPayloadAggregationKey(p.Env, p.Hostname, p.Version)
	payloadAgg, ok := b.aggregator[payloadAggKey]
	if !ok {
		var size int
		for _, s := range p.Stats {
			size += len(s.Stats)
		}
		payloadAgg = make(map[bucketAggregationKey]*aggregatedCounts, size)
		b.aggregator[payloadAggKey] = payloadAgg
	}
	for _, s := range p.Stats {
		for _, sb := range s.Stats {
			aggKey := newBucketAggregationKey(sb)
			agg, ok := payloadAgg[aggKey]
			if !ok {
				agg = &aggregatedCounts{}
				payloadAgg[aggKey] = agg
			}
			agg.hits += sb.Hits
			agg.errors += sb.Errors
			agg.duration += sb.Duration
		}
	}
}

func (b *bucket) flush() []pb.ClientStatsPayload {
	if b.numPayloads == 1 {
		return []pb.ClientStatsPayload{b.firstPayload}
	}
	return b.aggregationToPayloads()
}

func (b *bucket) aggregationToPayloads() []pb.ClientStatsPayload {
	res := make([]pb.ClientStatsPayload, 0, len(b.aggregator))
	for payloadKey, aggrCounts := range b.aggregator {
		stats := make([]pb.ClientGroupedStats, 0, len(aggrCounts))
		for aggrKey, counts := range aggrCounts {
			stats = append(stats, pb.ClientGroupedStats{
				Service:        aggrKey.service,
				Name:           aggrKey.name,
				Resource:       aggrKey.resource,
				HTTPStatusCode: aggrKey.statusCode,
				Type:           aggrKey.typ,
				Hits:           counts.hits,
				Errors:         counts.errors,
				Duration:       counts.duration,
			})
		}
		clientBuckets := []pb.ClientStatsBucket{
			{
				Start:    b.tsUnix * 1e9, // to nanosecond
				Duration: bucketDuration,
				Stats:    stats,
			}}
		res = append(res, pb.ClientStatsPayload{
			Hostname:         payloadKey.hostname,
			Env:              payloadKey.env,
			Version:          payloadKey.version,
			Stats:            clientBuckets,
			AgentAggregation: aggregatedCountPayload,
		})
	}
	return res
}

type payloadAggregationKey struct {
	env, hostname, version string
}

func newPayloadAggregationKey(env, hostname, version string) payloadAggregationKey {
	return payloadAggregationKey{env: env, hostname: hostname, version: version}
}

type bucketAggregationKey struct {
	service, name, resource, typ string
	synthetics                   bool
	statusCode                   uint32
}

func newBucketAggregationKey(b pb.ClientGroupedStats) bucketAggregationKey {
	return bucketAggregationKey{
		service:    b.Service,
		name:       b.Name,
		resource:   b.Resource,
		typ:        b.Type,
		synthetics: b.Synthetics,
		statusCode: b.HTTPStatusCode,
	}
}

func trimHitsErrors(p pb.ClientStatsPayload) pb.ClientStatsPayload {
	for _, s := range p.Stats {
		for i, b := range s.Stats {
			b.Hits = 0
			b.Errors = 0
			b.Duration = 0
			s.Stats[i] = b
		}
	}
	return p
}

// aggregate separately hits, errors, duration
// Distributions and TopLevelCount will stay on the initial payload
type aggregatedCounts struct {
	hits, errors, duration uint64
}
