// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trigger provides a very simple mechanism for watching a single
// incoming metric, tracking an exponential moving average of its value, and
// firing a callback when the average crosses a threshold. It is used as a
// prototype to trigger a metric lookback dump from a DogStatsD signal.
package trigger

import (
	"sync"
	"time"
)

// MovingAverage tracks an exponential moving average (EWMA) of a stream of
// values. It is not safe for concurrent use; callers must synchronize access.
type MovingAverage struct {
	alpha float64
	value float64
	count int
}

// NewMovingAverage creates an EWMA with the given smoothing factor. alpha is
// clamped to (0,1]: higher values react faster to new samples, 1 disables
// smoothing (the average always equals the latest value).
func NewMovingAverage(alpha float64) *MovingAverage {
	if alpha <= 0 || alpha > 1 {
		alpha = 1
	}
	return &MovingAverage{alpha: alpha}
}

// Update folds value into the moving average and returns the new average. The
// first observed value seeds the average directly.
func (m *MovingAverage) Update(value float64) float64 {
	if m.count == 0 {
		m.value = value
	} else {
		m.value = m.alpha*value + (1-m.alpha)*m.value
	}
	m.count++
	return m.value
}

// Value returns the current moving average (zero before any update).
func (m *MovingAverage) Value() float64 {
	return m.value
}

// Count returns how many values have been folded in.
func (m *MovingAverage) Count() int {
	return m.count
}

// Config controls a Watcher.
type Config struct {
	// MetricName is the exact metric name to watch. Samples with other names
	// are ignored.
	MetricName string
	// Threshold is the moving-average value at or above which the watcher
	// fires.
	Threshold float64
	// Alpha is the EWMA smoothing factor passed to NewMovingAverage.
	Alpha float64
	// Cooldown is the minimum time between successive fires. A zero cooldown
	// allows the watcher to fire on every qualifying sample.
	Cooldown time.Duration
}

// Watcher observes named samples, maintains a moving average of the watched
// metric, and fires onTrigger (in its own goroutine) when the average is at or
// above the threshold, subject to the cooldown. It is safe for concurrent use.
type Watcher struct {
	metricName string
	threshold  float64
	cooldown   time.Duration
	onTrigger  func()

	// now is injectable for deterministic tests; defaults to time.Now.
	now func() time.Time

	mu       sync.Mutex
	avg      *MovingAverage
	hasFired bool
	lastFire time.Time
	fires    uint64
}

// New creates a Watcher. It returns nil when the configuration is inert
// (empty metric name) or when onTrigger is nil, so callers can treat a nil
// Watcher as "disabled".
func New(cfg Config, onTrigger func()) *Watcher {
	if cfg.MetricName == "" || onTrigger == nil {
		return nil
	}
	return &Watcher{
		metricName: cfg.MetricName,
		threshold:  cfg.Threshold,
		cooldown:   cfg.Cooldown,
		onTrigger:  onTrigger,
		now:        time.Now,
		avg:        NewMovingAverage(cfg.Alpha),
	}
}

// MetricName returns the metric this watcher reacts to.
func (w *Watcher) MetricName() string {
	if w == nil {
		return ""
	}
	return w.metricName
}

// Observe folds a sample into the watcher. Samples whose name does not match
// the configured metric are ignored. It returns true when the observation
// caused the watcher to fire, in which case onTrigger has been dispatched in a
// new goroutine. Observe is a no-op (returns false) on a nil watcher.
func (w *Watcher) Observe(name string, value float64) bool {
	if w == nil || name != w.metricName {
		return false
	}

	w.mu.Lock()
	avg := w.avg.Update(value)
	now := w.now()
	fire := avg >= w.threshold && (!w.hasFired || now.Sub(w.lastFire) >= w.cooldown)
	if fire {
		w.hasFired = true
		w.lastFire = now
		w.fires++
	}
	w.mu.Unlock()

	if fire {
		go w.onTrigger()
	}
	return fire
}

// Average returns the current moving average of the watched metric.
func (w *Watcher) Average() float64 {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.avg.Value()
}

// Fires returns how many times the watcher has fired.
func (w *Watcher) Fires() uint64 {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fires
}
