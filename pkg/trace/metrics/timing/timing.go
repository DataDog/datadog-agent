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
	c   map[string]*counter // maps names to their aggregates
	ctx context.Context     // used for cancelling when auto-reporting
}

// NewSet returns a new Set containing the given metric names. The context is optional
// and can be used as cancellation for auto-reporting sets.
func NewSet(ctx context.Context, names ...string) *Set {
	set := Set{
		c:   make(map[string]*counter, len(names)),
		ctx: ctx,
	}
	for _, name := range names {
		set.c[name] = newCounter(name)
	}
	return &set
}

// Autoreport enables autoreporting of the Set at the given interval.
func (s *Set) Autoreport(interval time.Duration) {
	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
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

// Measure measures the time passed since, for the given name. It aggregates on each
// subsequent call until Report is called.
func (s *Set) Measure(name string, since time.Time) {
	ms := float64(time.Since(since) / time.Millisecond)
	c, ok := s.c[name]
	if !ok {
		panic(fmt.Sprintf("Set: key does not exist: %s", name))
	}
	c.add(ms)
}

// Report reports all of the Set's metrics to the statsd client.
func (s *Set) Report() {
	for _, c := range s.c {
		c.flush()
	}
}

type counter struct {
	// name specifies the name of this counter
	name string

	// mu synchronizes calls between add and flush, ensuring
	// that adding doesn't happen while flushing.
	mu sync.RWMutex

	count, max, sum *atomic.Float64
}

func newCounter(name string) *counter {
	return &counter{
		name:  name,
		count: atomic.NewFloat(0),
		max:   atomic.NewFloat(0),
		sum:   atomic.NewFloat(0),
	}
}

func (c *counter) add(v float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v > c.max.Load() {
		c.max.Store(v)
	}
	c.count.Add(1)
	c.sum.Add(v)
}

func (c *counter) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	metrics.Count(c.name+".count", int64(c.count.Load()), nil, 1)
	metrics.Gauge(c.name+".avg", c.sum.Load()/c.count.Load(), nil, 1)
	metrics.Gauge(c.name+".max", c.max.Load(), nil, 1)
	c.sum.Store(0)
	c.max.Store(0)
	c.count.Store(0)
}
