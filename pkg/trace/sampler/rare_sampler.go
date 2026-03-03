// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"time"

	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// ttlRenewalPeriod specifies the frequency at which we will upload cached entries.
	ttlRenewalPeriod = 1 * time.Minute
	// rareSamplerBurst sizes the token store used by the rate limiter.
	rareSamplerBurst = 50
	rareKey          = "_dd.rare"

	// MetricsRareHits is the metric name for the number of traces kept by the rare sampler.
	MetricsRareHits = "datadog.trace_agent.sampler.rare.hits"
	// MetricsRareMisses is the metric name for the number of traces missed by the rare sampler.
	MetricsRareMisses = "datadog.trace_agent.sampler.rare.misses"
	// MetricsRareShrinks is the metric name for the number of times the rare sampler has shrunk.
	MetricsRareShrinks = "datadog.trace_agent.sampler.rare.shrinks"
)

// RareSampler samples traces that are not caught by the Priority sampler.
// It ensures that we sample traces for each combination of
// (env, service, name, resource, error type, http status) seen on a top level or measured span
// for which we did not see any span with a priority > 0 (sampled by Priority).
// The resulting sampled traces will likely be incomplete and will be flagged with
// a exceptioKey metric set at 1.
type RareSampler struct {
	enabled *atomic.Bool
	hits    *atomic.Int64
	misses  *atomic.Int64
	shrinks *atomic.Int64
	mu      sync.RWMutex

	limiter     *rate.Limiter
	ttl         time.Duration
	cardinality int
	seen        map[Signature]*seenSpans
}

// NewRareSampler returns a NewRareSampler that ensures that we sample combinations
// of env, service, name, resource, http-status, error type for each top level or measured spans
func NewRareSampler(conf *config.AgentConfig) *RareSampler {
	e := &RareSampler{
		enabled:     atomic.NewBool(conf.RareSamplerEnabled),
		hits:        atomic.NewInt64(0),
		misses:      atomic.NewInt64(0),
		shrinks:     atomic.NewInt64(0),
		limiter:     rate.NewLimiter(rate.Limit(conf.RareSamplerTPS), rareSamplerBurst),
		ttl:         conf.RareSamplerCooldownPeriod,
		cardinality: conf.RareSamplerCardinality,
		seen:        make(map[Signature]*seenSpans),
	}
	return e
}

// Sample a trace and returns true if trace was sampled (should be kept)
func (e *RareSampler) Sample(now time.Time, t *pb.TraceChunk, env string) bool {
	if !e.enabled.Load() {
		return false
	}
	return e.handleTrace(now, env, t)
}

// SampleV1 samples a trace and returns true if trace was sampled (should be kept)
func (e *RareSampler) SampleV1(now time.Time, chunk *idx.InternalTraceChunk, env string) bool {
	if !e.enabled.Load() {
		return false
	}
	return e.handleTraceV1(now, env, chunk)
}

// SetEnabled marks the sampler as enabled or disabled
func (e *RareSampler) SetEnabled(enabled bool) {
	e.enabled.Store(enabled)
}

// IsEnabled returns whether the sampler is enabled
func (e *RareSampler) IsEnabled() bool {
	return e.enabled.Load()
}

func (e *RareSampler) handlePriorityTrace(now time.Time, env string, t *pb.TraceChunk, ttl time.Duration) {
	expire := now.Add(ttl)
	for _, s := range t.Spans {
		if !traceutil.HasTopLevel(s) && !traceutil.IsMeasured(s) {
			continue
		}
		e.addSpan(expire, env, s)
	}
}

func (e *RareSampler) handlePriorityTraceV1(now time.Time, env string, t *idx.InternalTraceChunk, ttl time.Duration) {
	expire := now.Add(ttl)
	for _, s := range t.Spans {
		if !traceutil.HasTopLevelMetricsV1(s) && !traceutil.IsMeasuredMetricsV1(s) {
			continue
		}
		e.addSpanV1(expire, env, s)
	}
}

func (e *RareSampler) handleTrace(now time.Time, env string, t *pb.TraceChunk) bool {
	var sampled bool
	for _, s := range t.Spans {
		if !traceutil.HasTopLevel(s) && !traceutil.IsMeasured(s) {
			continue
		}
		if sampled = e.sampleSpan(now, env, s); sampled {
			break
		}
	}

	if sampled {
		e.handlePriorityTrace(now, env, t, e.ttl)
	}
	return sampled
}

func (e *RareSampler) handleTraceV1(now time.Time, env string, t *idx.InternalTraceChunk) bool {
	var sampled bool
	for _, s := range t.Spans {
		if !traceutil.HasTopLevelMetricsV1(s) && !traceutil.IsMeasuredMetricsV1(s) {
			continue
		}
		if sampled = e.sampleSpanV1(now, env, s); sampled {
			break
		}
	}

	if sampled {
		e.handlePriorityTraceV1(now, env, t, e.ttl)
	}
	return sampled
}

// addSpan adds a span to the seenSpans with an expire time.
func (e *RareSampler) addSpan(expire time.Time, env string, s *pb.Span) {
	shardSig := ServiceSignature{env, s.Service}.Hash()
	ss := e.loadSeenSpans(shardSig)
	ss.add(expire, s)
}

// addSpan adds a span to the seenSpans with an expire time.
func (e *RareSampler) addSpanV1(expire time.Time, env string, s *idx.InternalSpan) {
	shardSig := ServiceSignature{env, s.Service()}.Hash()
	ss := e.loadSeenSpans(shardSig)
	ss.addV1(expire, s)
}

// sampleSpan samples a span if it's not in the seenSpan set. If the span is sampled
// it's added to the seenSpans set.
func (e *RareSampler) sampleSpan(now time.Time, env string, s *pb.Span) bool {
	var sampled bool
	shardSig := ServiceSignature{env, s.Service}.Hash()
	ss := e.loadSeenSpans(shardSig)
	sig := ss.sign(s)
	expire, ok := ss.getExpire(sig)
	if now.After(expire) || !ok {
		sampled = e.limiter.Allow()
		if sampled {
			ss.add(now.Add(e.ttl), s)
			e.hits.Inc()
			traceutil.SetMetric(s, rareKey, 1)
		} else {
			e.misses.Inc()
		}
	}
	return sampled
}

// sampleSpan samples a span if it's not in the seenSpan set. If the span is sampled
// it's added to the seenSpans set.
func (e *RareSampler) sampleSpanV1(now time.Time, env string, s *idx.InternalSpan) bool {
	var sampled bool
	shardSig := ServiceSignature{env, s.Service()}.Hash()
	ss := e.loadSeenSpans(shardSig)
	sig := ss.signV1(s)
	expire, ok := ss.getExpire(sig)
	if now.After(expire) || !ok {
		sampled = e.limiter.Allow()
		if sampled {
			ss.addV1(now.Add(e.ttl), s)
			e.hits.Inc()
			s.SetFloat64Attribute(rareKey, 1)
		} else {
			e.misses.Inc()
		}
	}
	return sampled
}

func (e *RareSampler) loadSeenSpans(shardSig Signature) *seenSpans {
	e.mu.RLock()
	s, ok := e.seen[shardSig]
	e.mu.RUnlock()
	if ok {
		return s
	}
	s = &seenSpans{
		expires:             make(map[spanHash]time.Time),
		totalSamplerShrinks: e.shrinks,
		cardinality:         e.cardinality,
	}
	e.mu.Lock()
	e.seen[shardSig] = s
	e.mu.Unlock()
	return s
}

func (e *RareSampler) report(statsd statsd.ClientInterface) {
	_ = statsd.Count(MetricsRareHits, e.hits.Swap(0), nil, 1)
	_ = statsd.Count(MetricsRareMisses, e.misses.Swap(0), nil, 1)
	_ = statsd.Gauge(MetricsRareShrinks, float64(e.shrinks.Load()), nil, 1)
}

// seenSpans keeps record of a set of spans.
type seenSpans struct {
	mu sync.RWMutex
	// expires contains expire time of each span seen.
	expires map[spanHash]time.Time
	// shrunk caracterize seenSpans when it's limited in size by capacityLimit.
	shrunk bool
	// totalSamplerShrinks is the reference to the total number of shrinks reported by RareSampler.
	totalSamplerShrinks *atomic.Int64
	// cardinality limits the number of spans considered per combination of (env, service).
	cardinality int
}

func (ss *seenSpans) add(expire time.Time, s *pb.Span) {
	sig := ss.sign(s)
	storedExpire, ok := ss.getExpire(sig)
	if ok && expire.Sub(storedExpire) < ttlRenewalPeriod {
		return
	}
	// slow path
	ss.mu.Lock()
	ss.expires[sig] = expire

	// if cardinality limit reached, shrink
	size := len(ss.expires)
	if size > ss.cardinality {
		ss.shrink()
	}
	ss.mu.Unlock()
}

func (ss *seenSpans) addV1(expire time.Time, s *idx.InternalSpan) {
	sig := ss.signV1(s)
	storedExpire, ok := ss.getExpire(sig)
	if ok && expire.Sub(storedExpire) < ttlRenewalPeriod {
		return
	}
	// slow path
	ss.mu.Lock()
	ss.expires[sig] = expire

	// if cardinality limit reached, shrink
	size := len(ss.expires)
	if size > ss.cardinality {
		ss.shrink()
	}
	ss.mu.Unlock()
}

// shrink limits the cardinality of signatures considered and the memory usage.
// This ensure that a service with high cardinality of resources does not consume
// all sampling tokens. The cardinality limit matches a backend limit.
// This function is not thread safe and should be called between locks
func (ss *seenSpans) shrink() {
	newExpires := make(map[spanHash]time.Time, ss.cardinality)
	for h, expire := range ss.expires {
		newExpires[h%spanHash(ss.cardinality)] = expire
	}
	ss.expires = newExpires
	ss.shrunk = true
	ss.totalSamplerShrinks.Inc()
}

func (ss *seenSpans) getExpire(h spanHash) (time.Time, bool) {
	ss.mu.RLock()
	expire, ok := ss.expires[h]
	ss.mu.RUnlock()
	return expire, ok
}

func (ss *seenSpans) sign(s *pb.Span) spanHash {
	h := computeSpanHash(s, "", true)
	if ss.shrunk {
		h = h % spanHash(ss.cardinality)
	}
	return h
}

func (ss *seenSpans) signV1(s *idx.InternalSpan) spanHash {
	h := computeSpanHashV1(s, "", true)
	if ss.shrunk {
		h = h % spanHash(ss.cardinality)
	}
	return h
}
