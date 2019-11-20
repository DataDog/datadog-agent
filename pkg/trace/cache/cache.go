// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package cache implements a set of caching mechanism for reassembling traces from a set of
// cached spans.
package cache

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

const (
	// MaxTraceIdle specifies the maximum interval that a trace is allowed to be idle before it is evicted.
	MaxTraceIdle = 1 * time.Minute
	// SweepPeriod specifies the period at which the reassembler cache is swept for idle traces.
	SweepPeriod = 30 * time.Second
)

// EvictReason specifies the reason that a trace was evicted by the reassembler.
type EvictReason int16

// String implements fmt.Stringer. It returns the eviction reason as a stastd-compatible tag.
func (er EvictReason) String() string {
	switch er {
	case EvictReasonRoot:
		return "reason:root"
	case EvictReasonSpace:
		return "reason:space"
	case EvictReasonIdle:
		return "reason:idle"
	case EvictReasonStopping:
		return "reason:stopping"
	default:
		return "reason:unknown"
	}
}

const (
	// EvictReasonRoot specifies that a trace was evicted because the root was found.
	EvictReasonRoot EvictReason = iota
	// EvictReasonSpace specifies that a trace was evicted because the reassembler's cache was running low on space.
	EvictReasonSpace
	// EvictReasonIdle specifies that a trace was evicted because it became idle.
	EvictReasonIdle
	// EvictReasonStopping specifies that a trace was evicted because the reassembler had to stop.
	EvictReasonStopping
)

// EvictedTrace holds information about a trace which was evicted by the reassembler.
type EvictedTrace struct {
	// Reason specifies the reason why the trace was evicted.
	Reason EvictReason
	// Spans holds all the spans that are part of this trace.
	Spans []*pb.Span
	// Source specifies information about the tracer where these spans originated from.
	Source *info.TagStats
}

// A Reassembler caches spans inside of it until they are formed into complete traces. A trace is considered complete
// based on serveral scenarios, such as idle time and root identification.
// Use NewReassembler to create a new Reassembler.
type Reassembler struct {
	out     chan<- *EvictedTrace // evict channel
	exit    chan struct{}        // exit notification
	wg      sync.WaitGroup       // wait on exit
	maxSize int                  // maximum allowed size in bytes
	evicted *keyCache            // tracks evicted traces to measure latecomers

	mu   sync.RWMutex             // guards below group
	ll   *list.List               // cache contents; most recently added to at the front
	keys map[uint64]*list.Element // maps keys to cache items
	size int                      // current cache size in bytes

	// stats
	evictedRoot  int64
	evictedSpace int64
	evictedIdle  int64
	evictedStop  int64
}

// NewReassembler creates a new reassembler which evicts traces through the given out channel and has a size limit
// of maxSize bytes.
func NewReassembler(out chan<- *EvictedTrace, maxSize int) *Reassembler {
	r := newReassembler(out, maxSize)
	r.exit = make(chan struct{})

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		tick := time.NewTicker(SweepPeriod)
		defer tick.Stop()
		r.sweepIdle(tick.C)
	}()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		tick := time.NewTicker(metricsFlushPeriod)
		defer tick.Stop()
		r.flushMetrics(tick.C)
	}()

	return r
}

func newReassembler(out chan<- *EvictedTrace, maxSize int) *Reassembler {
	return &Reassembler{
		out:     out,
		evicted: newKeyCache(maxKeyCacheSize),
		ll:      list.New(),
		keys:    make(map[uint64]*list.Element),
		maxSize: maxSize,
	}
}

// Item specifies an item to be added into the Reassembler.
type Item struct {
	Spans  []*pb.Span
	Source *info.TagStats

	key     uint64
	lastmod time.Time
	size    int
}

// Add adds the given item to the reassembler.
func (c *Reassembler) Add(item *Item) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addWithTime(time.Now(), item)
}

func (c *Reassembler) addWithTime(now time.Time, item *Item) {
	if c.ll == nil {
		panic("add to unstarted reassembler")
	}
	var roots []*pb.Span
	for _, span := range item.Spans {
		if isRoot(span) {
			roots = append(roots, span)
		}
		c.addSpan(now, span, item.Source)
	}
	for _, root := range roots {
		c.evictReasonRoot(root)
	}
	for c.size > c.maxSize {
		c.evictReasonSpace()
	}
}

func (c *Reassembler) addSpan(now time.Time, span *pb.Span, source *info.TagStats) {
	key := span.TraceID
	el, ok := c.keys[key]
	if ok {
		c.ll.MoveToFront(el)
	} else {
		el = c.ll.PushFront(&Item{
			key:    key,
			Source: source,
		})
		c.keys[key] = el
	}
	t := el.Value.(*Item)
	// TODO(gbbr): a user might've set a new sampling priority on some span.
	// Take these into account and add them onto a field in Item.
	t.Spans = append(t.Spans, span)
	t.lastmod = now
	size := span.Msgsize()
	t.size += size
	c.size += size
}

// Len returns the number of unevicted traces currently active in the reassembler.
func (c *Reassembler) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ll.Len()
}

const metricsFlushPeriod = 10 * time.Second

func (c *Reassembler) flushMetrics(tick <-chan time.Time) {
	for {
		select {
		case <-tick:
			c.mu.RLock()
			metrics.Gauge("datadog.trace_agent.reassembler.traces_cached", float64(c.ll.Len()), nil, 1)
			metrics.Gauge("datadog.trace_agent.reassembler.size_bytes", float64(c.size), nil, 1)
			c.mu.RUnlock()
			metrics.Gauge("datadog.trace_agent.reassembler.chan_fill", float64(len(c.out))/float64(cap(c.out)), nil, 1)
			metrics.Count("datadog.trace_agent.reassembler.traces_evicted", atomic.SwapInt64(&c.evictedRoot, 0), []string{"reason:root"}, 1)
			metrics.Count("datadog.trace_agent.reassembler.traces_evicted", atomic.SwapInt64(&c.evictedSpace, 0), []string{"reason:space"}, 1)
			metrics.Count("datadog.trace_agent.reassembler.traces_evicted", atomic.SwapInt64(&c.evictedIdle, 0), []string{"reason:idle"}, 1)
			metrics.Count("datadog.trace_agent.reassembler.traces_evicted", atomic.SwapInt64(&c.evictedStop, 0), []string{"reason:stop"}, 1)
		case <-c.exit:
			return
		}
	}
}

func (c *Reassembler) sweepIdle(tick <-chan time.Time) {
	for {
		select {
		case now := <-tick:
			c.mu.Lock()
			for {
				el := c.ll.Back()
				if el == nil {
					break
				}
				lastmod := el.Value.(*Item).lastmod
				if now.Sub(lastmod) > MaxTraceIdle {
					c.evictReasonIdle(el)
				} else {
					// it is safe to assume that we've reached a batch of traces
					// more recent than our threshold
					break
				}
			}
			c.mu.Unlock()
		case <-c.exit:
			return
		}
	}
}

func (c *Reassembler) evictReasonStopping() {
	for {
		el := c.ll.Front()
		if el == nil {
			break
		}
		c.evict(nil, el, EvictReasonStopping)
		atomic.AddInt64(&c.evictedStop, 1)
	}
}

func (c *Reassembler) evictReasonRoot(root *pb.Span) {
	key := root.TraceID
	el, ok := c.keys[key]
	if ok {
		c.evict(root, el, EvictReasonRoot)
		atomic.AddInt64(&c.evictedRoot, 1)
	}
}

func (c *Reassembler) evictReasonSpace() {
	if el := c.ll.Back(); el != nil {
		c.evict(nil, el, EvictReasonSpace)
		atomic.AddInt64(&c.evictedSpace, 1)
	}
}

func (c *Reassembler) evictReasonIdle(el *list.Element) {
	c.evict(nil, el, EvictReasonIdle)
	atomic.AddInt64(&c.evictedIdle, 1)
}

func (c *Reassembler) evict(root *pb.Span, el *list.Element, reason EvictReason) {
	defer timing.Since("datadog.trace_agent.reassembler.evict_time_ms", time.Now())

	item := el.Value.(*Item)
	c.out <- &EvictedTrace{
		Reason: reason,
		Spans:  item.Spans,
		Source: item.Source,
	}
	c.remove(el)
	if len(item.Spans) > 0 {
		if r, seen := c.evicted.Mark(item.Spans[0].TraceID, reason); seen {
			// We've seen this trace before. Mark the event and the original evict reason which was wrong.
			metrics.Count("datadog.trace_agent.reassembler.traces_broken", 1, []string{r.String()}, 1)
		}
	}
}

func (c *Reassembler) remove(el *list.Element) {
	trace := el.Value.(*Item)
	c.size -= trace.size
	c.ll.Remove(el)
	delete(c.keys, trace.key)
}

// Stop shutsdown an active reassembler and evicts the remaining items from the cache.
func (c *Reassembler) Stop() {
	if c.exit == nil {
		// not running
		return
	}
	select {
	case <-c.exit:
		// already stopped
		return
	default:
		// ok
	}
	close(c.exit)
	c.wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictReasonStopping()
	c.ll = nil
	close(c.out)
}

const tagRootSpan = "_dd.root"

func isRoot(span *pb.Span) bool {
	rule1 := span.ParentID == 0           // parent ID is 0, means root
	_, rule2 := span.Metrics[tagRootSpan] // client set root
	return rule1 || rule2
}
