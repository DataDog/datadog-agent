package stats

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

const (
	bucketDuration    = uint64(1e10) // 10s
	oldestBucketStart = 20 * time.Second
)

const (
	// set on a stat payload containing only distributions post aggregation
	keyDistributions = "distributions"
	// set on a stat payload containing counts (hit/error/duration) post aggregation
	keyCounts = "counts"
)

// ClientStatsAggregator aggregates client stats payloads on 1s buckets.
// If a single payload is received on a bucket, this Aggregator is a passthrough.
// If two or more payloads collide, their counts will be aggregated into one bucket.
// Multiple payloads will be sent:
// - Original payloads with their distributions will be sent with counts zeroed.
// - A single payload with the bucket aggregated counts will be sent.
// This ensures that all counts will have at most one point per second per agent for a specific granularity.
// While distributions are not tied to the agent.
type ClientStatsAggregator struct {
	In      chan pb.ClientStatsPayload
	out     chan pb.StatsPayload
	buckets map[uint64]*bucket // buckets used to aggregate client stats

	flushTicker   *time.Ticker
	oldestTs      uint64
	agentEnv      string
	agentHostname string

	exit chan struct{}
	done chan struct{}
}

// NewClientStatsAggregator initializes a new aggregator ready to be started
func NewClientStatsAggregator(conf *config.AgentConfig, out chan pb.StatsPayload) *ClientStatsAggregator {
	return &ClientStatsAggregator{
		flushTicker:   time.NewTicker(time.Second),
		In:            make(chan pb.ClientStatsPayload, 10),
		buckets:       make(map[uint64]*bucket, 20),
		out:           out,
		agentEnv:      conf.DefaultEnv,
		agentHostname: conf.Hostname,
		oldestTs:      uint64(time.Now().Add(-oldestBucketStart).Unix()),
		exit:          make(chan struct{}),
		done:          make(chan struct{}),
	}
}

// Start starts the aggregator.
func (a *ClientStatsAggregator) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		for {
			select {
			case t := <-a.flushTicker.C:
				a.flushOnTime(t)
			case input := <-a.In:
				a.add(time.Now(), input)
			case <-a.exit:
				a.flushAll()
				close(a.done)
				return
			}
		}
	}()
}

// Stop stops the aggregator. Calling Stop twice will panic.
func (a *ClientStatsAggregator) Stop() {
	close(a.exit)
	<-a.done
}

// flushOnTime flushes all buckets up to t, except the last one.
func (a *ClientStatsAggregator) flushOnTime(t time.Time) {
	flushTs := uint64(t.Unix()) - uint64(oldestBucketStart.Seconds())
	for ts := a.oldestTs; ts < flushTs; ts++ {
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
func (a *ClientStatsAggregator) getAggregationBucketTime(t time.Time, bucketStart uint64) (uint64, bool) {
	ts := bucketStart / 1e9 // conversion from nanosecond to second
	if ts < a.oldestTs {
		return a.oldestTs, true
	}
	now := uint64(t.Unix())
	if ts > now {
		return now, true
	}
	return ts, false
}

func (a *ClientStatsAggregator) add(now time.Time, p pb.ClientStatsPayload) {
	for _, clientBucket := range p.Stats {
		ts, shifted := a.getAggregationBucketTime(now, clientBucket.Start)
		if shifted {
			clientBucket.AgentTimeShift = int64(ts*1e9) - int64(clientBucket.Start)
			clientBucket.Start = ts * 1e9
		}
		b, ok := a.buckets[ts]
		if !ok {
			b = &bucket{ts: ts}
			a.buckets[ts] = b
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
	// first is the first payload matching the bucket. If a second payload matches the bucket
	// this field will be empty
	first pb.ClientStatsPayload
	// ts is the unix timestamp attached to the payload
	ts uint64
	// n counts the number of payloads matching the bucket
	n int
	// agg contains the aggregated Hits/Errors/Duration counts
	agg map[payloadAggregationKey]map[bucketAggregationKey]*aggregatedCounts
}

func (b *bucket) add(p pb.ClientStatsPayload) []pb.ClientStatsPayload {
	b.n++
	if b.n == 1 {
		b.first = p
		return nil
	}
	// if it's the second payload we flush the first payload with counts trimmed
	if b.n == 2 {
		first := b.first
		b.first = pb.ClientStatsPayload{}
		b.agg = make(map[payloadAggregationKey]map[bucketAggregationKey]*aggregatedCounts, 2)
		b.aggregateCounts(first)
		b.aggregateCounts(p)
		return []pb.ClientStatsPayload{trimCounts(first), trimCounts(p)}
	}
	b.aggregateCounts(p)
	return []pb.ClientStatsPayload{trimCounts(p)}
}

func (b *bucket) aggregateCounts(p pb.ClientStatsPayload) {
	payloadAggKey := newPayloadAggregationKey(p.Env, p.Hostname, p.Version)
	payloadAgg, ok := b.agg[payloadAggKey]
	if !ok {
		var size int
		for _, s := range p.Stats {
			size += len(s.Stats)
		}
		payloadAgg = make(map[bucketAggregationKey]*aggregatedCounts, size)
		b.agg[payloadAggKey] = payloadAgg
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
	if b.n == 1 {
		return []pb.ClientStatsPayload{b.first}
	}
	return b.aggregationToPayloads()
}

func (b *bucket) aggregationToPayloads() []pb.ClientStatsPayload {
	res := make([]pb.ClientStatsPayload, 0, len(b.agg))
	for payloadKey, aggrCounts := range b.agg {
		stats := make([]pb.ClientGroupedStats, 0, len(aggrCounts))
		for aggrKey, counts := range aggrCounts {
			stats = append(stats, pb.ClientGroupedStats{
				Service:        aggrKey.service,
				Name:           aggrKey.name,
				Resource:       aggrKey.resource,
				HTTPStatusCode: aggrKey.statusCode,
				Type:           aggrKey.typ,
				Synthetics:     aggrKey.synthetics,
				Hits:           counts.hits,
				Errors:         counts.errors,
				Duration:       counts.duration,
			})
		}
		clientBuckets := []pb.ClientStatsBucket{
			{
				Start:    b.ts * 1e9, // to nanosecond
				Duration: bucketDuration,
				Stats:    stats,
			}}
		res = append(res, pb.ClientStatsPayload{
			Hostname:         payloadKey.hostname,
			Env:              payloadKey.env,
			Version:          payloadKey.version,
			Stats:            clientBuckets,
			AgentAggregation: keyCounts,
		})
	}
	return res
}

// payloadAggregationKey and bucketAggregationKey contain dimensions used
// to aggregate statistics. When adding or removing fields, update accordingly
// the Aggregation used by the concentrator.
type payloadAggregationKey struct {
	env, hostname, version string
}

type bucketAggregationKey struct {
	service, name, resource, typ string
	synthetics                   bool
	statusCode                   uint32
}

func newPayloadAggregationKey(env, hostname, version string) payloadAggregationKey {
	return payloadAggregationKey{env: env, hostname: hostname, version: version}
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

func trimCounts(p pb.ClientStatsPayload) pb.ClientStatsPayload {
	p.AgentAggregation = keyDistributions
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
