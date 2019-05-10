// Package timing is used to aggregate timing calls within hotpaths to avoid using
// repeated statsd calls. The package has a default set that reports at 10 second
// intervals and can be used directly. If a different behaviour or reporting pattern
// is desired, a custom Set may be created.
package timing

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/atomic"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
)

// AutoreportInterval specifies the interval at which the default set reports.
const AutoreportInterval = 10 * time.Second

var (
	defaultSet = NewSet()
	stopReport = defaultSet.Autoreport(AutoreportInterval)
)

// Since records the duration for the given metric name as time passed since start.
// It uses the default set which is reported at 10 second intervals.
func Since(name string, start time.Time) { defaultSet.Since(name, start) }

// Flush reports the default set. It can be useful to call when the program
// exits prematurely.
func Flush() { stopReport() }

// Set represents a set of metrics that can be used for timing. Use NewSet
// to create one and Report (or Autoreport) to submit metrics. It is safe for
// concurrent use.
type Set struct {
	mu sync.RWMutex        // guards c
	c  map[string]*counter // maps names to their aggregates
}

// NewSet returns a new Set, optionally initialized with the given metric names.
func NewSet(names ...string) *Set {
	set := Set{
		c: make(map[string]*counter, len(names)),
	}
	for _, name := range names {
		set.c[name] = newCounter(name)
	}
	return &set
}

// Autoreport enables autoreporting of the Set at the given interval. It returns a
// cancellation function.
func (s *Set) Autoreport(interval time.Duration) (cancelFunc func()) {
	stop := make(chan struct{})
	go func() {
		defer close(stop)
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				s.Report()
			case <-stop:
				s.Report()
				return
			}
		}
	}()
	var once sync.Once // avoid panics
	return func() {
		once.Do(func() {
			stop <- struct{}{}
			<-stop
		})
	}
}

// Since records the duration for the given metric name as time passed since start.
// If name does not exist, as defined by NewSet, it creates it.
func (s *Set) Since(name string, start time.Time) {
	s.mu.RLock()
	c, ok := s.c[name]
	s.mu.RUnlock()
	if !ok {
		c = newCounter(name)
		s.mu.Lock()
		s.c[name] = c
		s.mu.Unlock()
	}
	ms := time.Since(start) / time.Millisecond
	c.add(float64(ms))
}

// Report reports all of the Set's metrics to the statsd client.
func (s *Set) Report() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.c {
		c.flush()
	}
}

type counter struct {
	// name specifies the name of this counter
	name string

	// mu keeps count and sum in sync, ensuring that average calculations
	// are correct between add and flush calls.
	mu    sync.RWMutex
	sum   *atomic.Float64
	count *atomic.Float64

	max *atomic.Float64
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
	if v > c.max.Load() {
		c.max.Store(v)
	}
	c.mu.RLock()
	c.count.Add(1)
	c.sum.Add(v)
	c.mu.RUnlock()
}

func (c *counter) flush() {
	metrics.Count(c.name+".count", int64(c.count.Load()), nil, 1)
	metrics.Gauge(c.name+".max", c.max.Load(), nil, 1)
	c.mu.Lock()
	metrics.Gauge(c.name+".avg", c.sum.Load()/c.count.Load(), nil, 1)
	c.sum.Store(0)
	c.count.Store(0)
	c.mu.Unlock()
	c.max.Store(0)
}
