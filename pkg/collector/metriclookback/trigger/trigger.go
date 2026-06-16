// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trigger provides a very simple mechanism for watching a single
// incoming metric, tracking an exponential moving average of its value, and
// firing a callback when the average crosses a threshold. It is used as a
// prototype to trigger metric lookback dumps from a DogStatsD signal.
package trigger

import (
	"sync"
	"time"
)

const (
	// DefaultDumpInterval is used when Config.DumpInterval is not set. The Agent
	// config supplies an explicit default too; this package-level fallback keeps
	// direct uses safe from tight retry loops.
	DefaultDumpInterval = 10 * time.Second
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

// DumpFunc dumps retained lookback samples whose original timestamps fall in
// the inclusive [from, to] window. A zero from or to leaves that side of the
// window unbounded.
type DumpFunc func(from, to time.Time) (int, error)

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
	// Cooldown is the minimum time between successive dump sessions. A zero
	// cooldown allows the watcher to fire whenever no dump session is already
	// active.
	Cooldown time.Duration
	// PreWindow is how far before the trigger timestamp each dump session starts.
	PreWindow time.Duration
	// PostWindow is how far after the trigger timestamp each dump session ends.
	PostWindow time.Duration
	// DumpInterval is how often an active dump session wakes up to send newly
	// eligible samples. When unset, DefaultDumpInterval is used.
	DumpInterval time.Duration
	// SendDelay is the minimum age a sample timestamp must have before it is
	// dumped. This lets lookback data arrive after the normal metric pipeline for
	// the same timestamp.
	SendDelay time.Duration
}

// Watcher observes named samples, maintains a moving average of the watched
// metric, and starts a windowed dump session when the average is at or above the
// threshold, subject to the cooldown. It is safe for concurrent use.
type Watcher struct {
	metricName   string
	threshold    float64
	cooldown     time.Duration
	preWindow    time.Duration
	postWindow   time.Duration
	dumpInterval time.Duration
	sendDelay    time.Duration
	dump         DumpFunc

	// now and sleep are injectable for deterministic tests; they default to
	// time.Now and time.Sleep.
	now   func() time.Time
	sleep func(time.Duration)

	mu            sync.Mutex
	avg           *MovingAverage
	hasFired      bool
	lastFire      time.Time
	fires         uint64
	sessionActive bool
}

// New creates a Watcher. It returns nil when the configuration is inert (empty
// metric name) or when dump is nil, so callers can treat a nil Watcher as
// "disabled".
func New(cfg Config, dump DumpFunc) *Watcher {
	if cfg.MetricName == "" || dump == nil {
		return nil
	}
	cfg = normalizeConfig(cfg)
	return &Watcher{
		metricName:   cfg.MetricName,
		threshold:    cfg.Threshold,
		cooldown:     cfg.Cooldown,
		preWindow:    cfg.PreWindow,
		postWindow:   cfg.PostWindow,
		dumpInterval: cfg.DumpInterval,
		sendDelay:    cfg.SendDelay,
		dump:         dump,
		now:          time.Now,
		sleep:        time.Sleep,
		avg:          NewMovingAverage(cfg.Alpha),
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.Cooldown < 0 {
		cfg.Cooldown = 0
	}
	if cfg.PreWindow < 0 {
		cfg.PreWindow = 0
	}
	if cfg.PostWindow < 0 {
		cfg.PostWindow = 0
	}
	if cfg.DumpInterval <= 0 {
		cfg.DumpInterval = DefaultDumpInterval
	}
	if cfg.SendDelay < 0 {
		cfg.SendDelay = 0
	}
	return cfg
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
// started a dump session, in which case the session runs in a new goroutine.
// Observe is a no-op (returns false) on a nil watcher.
func (w *Watcher) Observe(name string, value float64) bool {
	if w == nil || name != w.metricName {
		return false
	}

	w.mu.Lock()
	avg := w.avg.Update(value)
	now := w.now()
	fire := avg >= w.threshold && !w.sessionActive && (!w.hasFired || now.Sub(w.lastFire) >= w.cooldown)
	if fire {
		w.hasFired = true
		w.sessionActive = true
		w.lastFire = now
		w.fires++
	}
	w.mu.Unlock()

	if fire {
		go w.startDumpSession(now)
	}
	return fire
}

func (w *Watcher) startDumpSession(triggeredAt time.Time) {
	defer w.finishDumpSession()
	w.runDumpSession(triggeredAt)
}

func (w *Watcher) finishDumpSession() {
	w.mu.Lock()
	w.sessionActive = false
	w.mu.Unlock()
}

func (w *Watcher) runDumpSession(triggeredAt time.Time) {
	if w == nil {
		return
	}

	startMicro := triggeredAt.Add(-w.preWindow).UnixMicro()
	endMicro := triggeredAt.Add(w.postWindow).UnixMicro()
	nextFromMicro := startMicro

	for nextFromMicro <= endMicro {
		throughMicro := w.now().Add(-w.sendDelay).UnixMicro()
		if throughMicro > endMicro {
			throughMicro = endMicro
		}
		if throughMicro >= nextFromMicro {
			_, _ = w.dump(time.UnixMicro(nextFromMicro), time.UnixMicro(throughMicro))
			nextFromMicro = throughMicro + 1
			continue
		}

		w.sleep(w.dumpInterval)
	}
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

// Fires returns how many dump sessions this watcher has started.
func (w *Watcher) Fires() uint64 {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fires
}
