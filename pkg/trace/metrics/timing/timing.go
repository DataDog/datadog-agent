// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package timing is used to aggregate timing calls within hotpaths to avoid using
// repeated statsd calls. The package has a default set that reports at 10 second
// intervals and can be used directly. If a different behaviour or reporting pattern
// is desired, a custom Set may be created.
package timing

import (
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
)

// autoreportInterval specifies the interval at which the default set reports.
var autoreportInterval = 10 * time.Second

var (
	defaultSet *set
)

// Since records the duration for the given metric name as time passed since start.
// It uses the default set which is reported at 10 second intervals.
func Since(name string, start time.Time) {
	if defaultSet == nil {
		log.Error("Timing hasn't been initialized, trace-agent metrics will be missing")
		return
	}
	defaultSet.Since(name, start)
}

// Start initializes autoreporting of timing metrics.
func Start(statsd metrics.StatsClient) {
	defaultSet = newSet(statsd)
	defaultSet.autoreport(autoreportInterval)
}

// Stop permanently stops the default set from auto-reporting and flushes any remaining
// metrics. It can be useful to call when the program exits to ensure everything is
// submitted.
func Stop() {
	if defaultSet == nil {
		return
	}
	defaultSet.stop()
}

// newSet returns a new, ready to use Set.
func newSet(statsd metrics.StatsClient) *set {
	return &set{
		c:      make(map[string]*counter),
		close:  make(chan struct{}),
		statsd: statsd,
	}
}

// Set represents a set of metrics that can be used for timing. Use NewSet to initialize
// a new Set. Use Report (or Autoreport) to submit metrics. Set is safe for concurrent use.
type set struct {
	mu        sync.RWMutex        // guards c
	c         map[string]*counter // maps names to their aggregates
	close     chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	statsd    metrics.StatsClient
}

// autoreport enables autoreporting of the Set at the given interval. It returns a
// cancellation function.
func (s *set) autoreport(interval time.Duration) {
	s.startOnce.Do(func() {
		go func() {
			tick := time.NewTicker(interval)
			defer tick.Stop()
			for {
				select {
				case <-tick.C:
					s.report()
				case <-s.close:
					s.report()
					return
				}
			}
		}()
	})
}

func (s *set) stop() {
	s.stopOnce.Do(func() {
		close(s.close)
	})
}

// Since records the duration for the given metric name as *time passed since start*.
// If name does not exist, as defined by NewSet, it creates it.
func (s *set) Since(name string, start time.Time) {
	ms := time.Since(start) / time.Millisecond
	s.getCounter(name).add(float64(ms))
}

// getCounter returns the counter with the given name, initializing any uninitialized
// fields of s.
func (s *set) getCounter(name string) *counter {
	s.mu.RLock()
	c, ok := s.c[name]
	s.mu.RUnlock()
	if !ok {
		// initialize a new counter
		s.mu.Lock()
		defer s.mu.Unlock()
		if c, ok := s.c[name]; ok {
			// another goroutine already did it
			return c
		}
		s.c[name] = newCounter(name)
		c = s.c[name]
	}
	return c
}

// Report reports all of the Set's metrics to the statsd client.
func (s *set) report() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.c {
		c.flush(s.statsd)
	}
}

type counter struct {
	// name specifies the name of this counter
	name string

	// mu guards the below field from changes during flushing.
	mu    sync.RWMutex
	sum   *atomic.Float64
	count *atomic.Float64
	max   *atomic.Float64
}

func newCounter(name string) *counter {
	return &counter{
		name:  name,
		count: atomic.NewFloat64(0),
		max:   atomic.NewFloat64(0),
		sum:   atomic.NewFloat64(0),
	}
}

func (c *counter) add(v float64) {
	c.mu.RLock()
	if v > c.max.Load() {
		c.max.Store(v)
	}
	c.count.Add(1)
	c.sum.Add(v)
	c.mu.RUnlock()
}

func (c *counter) flush(statsd metrics.StatsClient) {
	c.mu.Lock()
	count := c.count.Swap(0)
	sum := c.sum.Swap(0)
	max := c.max.Swap(0)
	c.mu.Unlock()
	statsd.Count(c.name+".count", int64(count), nil, 1)
	statsd.Gauge(c.name+".max", max, nil, 1)
	statsd.Gauge(c.name+".avg", sum/count, nil, 1)
}
