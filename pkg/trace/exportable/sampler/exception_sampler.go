package sampler

import (
	"sync"
	"sync/atomic"
	"time"

	metricsClient "github.com/DataDog/datadog-agent/pkg/trace/exportable/metrics/client"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/traceutil"
	"golang.org/x/time/rate"
)

const (
	// cardinalityLimit limits the number of spans considered per combination of (env, service).
	cardinalityLimit = 1000
	// defaultTTL limits the frequency at which we sample a same span (env, service, name, rsc, ...).
	defaultTTL = 2 * time.Minute
	// priorityTTL allows to blacklist p1 spans that are sampled entirely, for this period.
	priorityTTL = 10 * time.Minute
	// ttlRenewalPeriod specifies the frequency at which we will upload cached entries.
	ttlRenewalPeriod = 1 * time.Minute
	// exceptionSamplerTPS traces per second allowed by the rate limiter.
	exceptionSamplerTPS = 5
	// exceptionSamplerBurst sizes the token store used by the rate limiter.
	exceptionSamplerBurst = 50
	exceptionKey          = "_dd.exception"
)

// ExceptionSampler samples traces that are not caught by the Priority sampler.
// It ensures that we sample traces for each combination of
// (env, service, name, resource, error type, http status) seen on a top level or measured span
// for which we did not see any span with a priority > 0 (sampled by Priority).
// The resulting sampled traces will likely be incomplete and will be flagged with
// a exceptioKey metric set at 1.
type ExceptionSampler struct {
	// Variables access through the 'atomic' package must be 64bits aligned.
	hits    int64
	misses  int64
	shrinks int64
	mu      sync.RWMutex

	tickStats *time.Ticker
	limiter   *rate.Limiter
	seen      map[Signature]*seenSpans
}

// NewExceptionSampler returns a NewExceptionSampler that ensures that we sample combinations
// of env, service, name, resource, http-status, error type for each top level or measured spans
func NewExceptionSampler() *ExceptionSampler {
	e := &ExceptionSampler{
		limiter:   rate.NewLimiter(exceptionSamplerTPS, exceptionSamplerBurst),
		seen:      make(map[Signature]*seenSpans),
		tickStats: time.NewTicker(10 * time.Second),
	}
	go func() {
		for range e.tickStats.C {
			e.report()
		}
	}()
	return e
}

// Add samples a trace and returns true if trace was sampled (should be kept)
func (e *ExceptionSampler) Add(env string, root *pb.Span, t pb.Trace) (sampled bool) {
	return e.add(time.Now(), env, root, t)
}

func (e *ExceptionSampler) add(now time.Time, env string, root *pb.Span, t pb.Trace) (sampled bool) {
	if priority, ok := GetSamplingPriority(root); priority > 0 && ok {
		e.handlePriorityTrace(now, env, t)
		return false
	}
	return e.handleTrace(now, env, t)
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

// addSpan adds a span to the seenSpans with an expire time.
func (e *ExceptionSampler) addSpan(expire time.Time, env string, s *pb.Span) {
	shardSig := ServiceSignature{env, s.Service}.Hash()
	ss := e.loadSeenSpans(shardSig)
	ss.add(expire, s)
}

// sampleSpan samples a span if it's not in the seenSpan set. If the span is sampled
// it's added to the seenSpans set.
func (e *ExceptionSampler) sampleSpan(now time.Time, env string, s *pb.Span) bool {
	var sampled bool
	shardSig := ServiceSignature{env, s.Service}.Hash()
	ss := e.loadSeenSpans(shardSig)
	sig := ss.sign(s)
	expire, ok := ss.getExpire(sig)
	if now.After(expire) || !ok {
		sampled = e.limiter.Allow()
		if sampled {
			ss.add(now.Add(defaultTTL), s)
			atomic.AddInt64(&e.hits, 1)
			traceutil.SetMetric(s, exceptionKey, 1)
		} else {
			atomic.AddInt64(&e.misses, 1)
		}
	}
	return sampled
}

func (e *ExceptionSampler) loadSeenSpans(shardSig Signature) *seenSpans {
	e.mu.RLock()
	s, ok := e.seen[shardSig]
	e.mu.RUnlock()
	if ok {
		return s
	}
	s = &seenSpans{expires: make(map[spanHash]time.Time), totalSamplerShrinks: &e.shrinks}
	e.mu.Lock()
	e.seen[shardSig] = s
	e.mu.Unlock()
	return s
}

func (e *ExceptionSampler) report() {
	metricsClient.Count("datadog.trace_agent.sampler.exception.hits", atomic.SwapInt64(&e.hits, 0), nil, 1)
	metricsClient.Count("datadog.trace_agent.sampler.exception.misses", atomic.SwapInt64(&e.misses, 0), nil, 1)
	metricsClient.Gauge("datadog.trace_agent.sampler.exception.shrinks", float64(atomic.LoadInt64(&e.shrinks)), nil, 1)
}

// seenSpans keeps record of a set of spans.
type seenSpans struct {
	mu sync.RWMutex
	// expires contains expire time of each span seen.
	expires map[spanHash]time.Time
	// shrunk caracterize seenSpans when it's limited in size by capacityLimit.
	shrunk bool
	// totalSamplerShrinks is the reference to the total number of shrinks reported by ExceptionSampler.
	totalSamplerShrinks *int64
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
	if size > cardinalityLimit {
		ss.shrink()
	}
	ss.mu.Unlock()
}

// shrink limits the cardinality of signatures considered and the memory usage.
// This ensure that a service with high cardinality of resources does not consume
// all sampling tokens. The cardinality limit matches a backend limit.
// This function is not thread safe and should be called between locks
func (ss *seenSpans) shrink() {
	newExpires := make(map[spanHash]time.Time, cardinalityLimit)
	for h, expire := range ss.expires {
		newExpires[h%spanHash(cardinalityLimit)] = expire
	}
	ss.expires = newExpires
	ss.shrunk = true
	atomic.AddInt64(ss.totalSamplerShrinks, 1)
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
		h = h % spanHash(cardinalityLimit)
	}
	return h
}
