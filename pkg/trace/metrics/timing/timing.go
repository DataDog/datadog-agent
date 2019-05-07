package timing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/atomic"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
)

// Set represents a set of metrics that can be used for timing. Use NewSet
// to create a set. It is safe for concurrent use.
type Set struct {
	c   map[string]*agg // maps names to their aggregates
	ctx context.Context // used for cancelling when auto-reporting
	mu  sync.RWMutex    // synchronizes calls between Report and Time
}

type agg struct {
	count, max, avg, sum *atomic.Float64
}

// NewSet returns a new Set containing the given metric names. The context is optional
// and can be used as cancellation for auto-reporting sets.
func NewSet(ctx context.Context, names ...string) *Set {
	set := Set{
		c:   make(map[string]*agg, len(names)),
		ctx: ctx,
	}
	for _, name := range names {
		set.c[name] = &agg{
			count: atomic.NewFloat(0),
			max:   atomic.NewFloat(0),
			sum:   atomic.NewFloat(0),
			avg:   atomic.NewFloat(0),
		}
	}
	return &set
}

// Autoreport enables autoreporting of the Set at the given interval.
func (s *Set) Autoreport(interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	go func() {
		for {
			select {
			case <-tick.C:
				s.Report()
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

// Time measures the time passed since, for the given name. It aggregates on each
// subsequent call until Report is called.
func (s *Set) Time(name string, since time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ms := float64(time.Since(since) / time.Millisecond)
	c, ok := s.c[name]
	if !ok {
		panic(fmt.Sprintf("Set: key does not exist: %s", name))
	}
	if ms > c.max.Load() {
		c.max.Store(ms)
	}
	c.count.Add(1)
	c.sum.Add(ms)
	c.avg.Store(c.sum.Load() / c.count.Load())
}

// Report reports all of the Set's metrics to the statsd client.
func (s *Set) Report() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, agg := range s.c {
		metrics.Gauge(name+".count", agg.count.Swap(0), nil, 1)
		metrics.Gauge(name+".max", agg.max.Swap(0), nil, 1)
		metrics.Gauge(name+".avg", agg.avg.Swap(0), nil, 1)
		agg.sum.Store(0)
	}
}
