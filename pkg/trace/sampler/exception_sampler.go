package sampler

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"golang.org/x/time/rate"
)

const (
	cardinalityLimit      = 1000
	defaultTTL            = 2 * time.Minute
	priorityTTL           = 10 * time.Minute
	ttlRenewalPeriod      = 1 * time.Minute
	exceptionSamplerTPS   = 5
	exceptionSamplerBurst = 50
)

// ExceptionSampler samples traces that are not caught by the Priority sampler.
// It ensures that we sample traces for each combination of
// (env, service, name, resource, error type, http status) seen on a measured span.
// The resulting sampled traces will likely be incomplete.
type ExceptionSampler struct {
	mu           sync.RWMutex
	limiter      *rate.Limiter
	spanSeenSets map[Signature]*spanSeenSet

	tickStats *time.Ticker
	hits      int64
	misses    int64
}

// NewExceptionSampler returns a NewExceptionSampler that samples rare traces
// not caught by Priority sampler.
func NewExceptionSampler() *ExceptionSampler {
	return &ExceptionSampler{
		limiter:      rate.NewLimiter(exceptionSamplerTPS, exceptionSamplerBurst),
		spanSeenSets: make(map[Signature]*spanSeenSet),
		tickStats:    time.NewTicker(10 * time.Second),
	}
}

// Add samples a trace and returns true if trace was sampled (should be kept)
func (e *ExceptionSampler) Add(now time.Time, env string, root *pb.Span, t pb.Trace) (sampled bool) {
	if priority, ok := GetSamplingPriority(root); priority > 0 && ok {
		e.handlePriorityTrace(now, env, t)
		return false
	}
	return e.handleTrace(now, env, t)
}

// Start starts reporting stats
func (e *ExceptionSampler) Start() {
	go func() {
		for _ = range e.tickStats.C {
			e.report()
		}
	}()
}

// Stop stops reporting stats
func (e *ExceptionSampler) Stop() {
	e.tickStats.Stop()
}

func (e *ExceptionSampler) handlePriorityTrace(now time.Time, env string, t pb.Trace) {
	expire := now.Add(priorityTTL)
	for _, s := range t {
		if !traceutil.HasTopLevel(s) && !traceutil.IsMeasured(s) {
			continue
		}
		e.addSpan(expire, env, s)
	}
}

func (e *ExceptionSampler) handleTrace(now time.Time, env string, t pb.Trace) bool {
	var sampled bool
	expire := now.Add(defaultTTL)
	for _, s := range t {
		if !traceutil.HasTopLevel(s) && !traceutil.IsMeasured(s) {
			continue
		}
		if !sampled {
			sampled = e.sampleSpan(now, env, s)
			continue
		}
		e.addSpan(expire, env, s)
	}
	return sampled
}

func (e *ExceptionSampler) addSpan(expire time.Time, env string, s *pb.Span) {
	shardSig := ServiceSignature{env, s.Service}.Hash()
	b := e.loadSpanSeenSet(shardSig)
	b.add(expire, s)
}

func (e *ExceptionSampler) sampleSpan(now time.Time, env string, s *pb.Span) bool {
	var sampled bool
	shardSig := ServiceSignature{env, s.Service}.Hash()
	b := e.loadSpanSeenSet(shardSig)
	sig := b.sign(s)
	expire, ok := b.getExpire(sig)
	if now.After(expire) || !ok {
		sampled = e.limiter.Allow()
		if sampled {
			b.add(now.Add(defaultTTL), s)
			atomic.AddInt64(&e.hits, 1)
		} else {
			atomic.AddInt64(&e.misses, 1)
		}
	}
	return sampled
}

func (e *ExceptionSampler) loadSpanSeenSet(shardSig Signature) *spanSeenSet {
	e.mu.RLock()
	s, ok := e.spanSeenSets[shardSig]
	e.mu.RUnlock()
	if ok {
		return s
	}
	s = &spanSeenSet{expires: make(map[spanHash]time.Time)}
	e.mu.Lock()
	e.spanSeenSets[shardSig] = s
	e.mu.Unlock()
	return s
}

func (e *ExceptionSampler) report() {
	metrics.Count("datadog.trace_agent.trace_sampler.exception.hits", atomic.SwapInt64(&e.hits, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sampler.exception.misses", atomic.SwapInt64(&e.misses, 0), nil, 1)
}

type spanSeenSet struct {
	mu      sync.RWMutex
	expires map[spanHash]time.Time
	shrunk  bool
}

func (ss *spanSeenSet) add(expire time.Time, s *pb.Span) {
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
	if size > cardinalityLimit {
		ss.shrink()
	}
	ss.mu.Unlock()
}

// shrink limits the cardinality of signatures considered and the memory usage.
// This ensure that a service with high cardinality of resources does not consume
// all sampling tokens. The cardinality limit matches a backend limit.
// This function is not thread safe and should be called between locks
func (ss *spanSeenSet) shrink() {
	newExpires := make(map[spanHash]time.Time, cardinalityLimit)
	for h, expire := range ss.expires {
		newExpires[h%spanHash(cardinalityLimit)] = expire
	}
	ss.expires = newExpires
	ss.shrunk = true
}

func (ss *spanSeenSet) getExpire(h spanHash) (time.Time, bool) {
	ss.mu.RLock()
	expire, ok := ss.expires[h]
	ss.mu.RUnlock()
	return expire, ok
}

func (ss *spanSeenSet) sign(s *pb.Span) spanHash {
	h := computeSpanHash(s, "", true)
	if ss.shrunk {
		h = h % spanHash(cardinalityLimit)
	}
	return h
}
