// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
)

const (
	// maxHistorySamples holds up to 10 hours of 1-second samples per component.
	maxHistorySamples = 10 * 3600
	// SaturationThreshold is the raw ratio above which a component is considered saturated.
	SaturationThreshold = 0.90
)

type histSample struct {
	tsNano int64   // UnixNano timestamp
	value  float64 // raw (pre-EWMA) utilization ratio for this 1-second window
}

// WindowStats contains pre-computed statistics over several time windows.
// All values are fractions in [0,1]; multiply by 100 for percentages.
type WindowStats struct {
	Avg5m  float64
	Max5m  float64
	Avg30m float64
	Max30m float64
	Max2h  float64
	Max5h  float64
	Max10h float64
	// Saturated1m is total time within the last 1 minute where value >= SaturationThreshold.
	Saturated1m time.Duration
	// Saturated30m is total time within the last 30 minutes where value >= SaturationThreshold.
	// Since samples are 1 second apart, each qualifying sample counts as 1 second.
	Saturated30m     time.Duration
	LastSaturatedAt  time.Time
	HasLastSaturated bool
}

// rollingHistory is a fixed-capacity ring buffer of 1-second utilization samples.
// It is owned by a single TelemetryUtilizationMonitor and its mutex guards concurrent
// reads from the status builder.
type rollingHistory struct {
	mu   sync.Mutex
	buf  []histSample
	head int // index of the next write slot
	size int // number of valid entries (≤ maxHistorySamples)
}

func newRollingHistory() *rollingHistory {
	return &rollingHistory{
		buf: make([]histSample, maxHistorySamples),
	}
}

func (h *rollingHistory) add(ts time.Time, value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf[h.head] = histSample{tsNano: ts.UnixNano(), value: value}
	h.head = (h.head + 1) % maxHistorySamples
	if h.size < maxHistorySamples {
		h.size++
	}
}

// allStats computes all window statistics in a single backwards scan so the
// caller doesn't need to re-acquire the lock per window.
func (h *rollingHistory) allStats(now time.Time) WindowStats {
	h.mu.Lock()
	defer h.mu.Unlock()

	c1m := now.Add(-1 * time.Minute).UnixNano()
	c5m := now.Add(-5 * time.Minute).UnixNano()
	c30m := now.Add(-30 * time.Minute).UnixNano()
	c2h := now.Add(-2 * time.Hour).UnixNano()
	c5h := now.Add(-5 * time.Hour).UnixNano()
	c10h := now.Add(-10 * time.Hour).UnixNano()

	var (
		sum5m, sum30m        float64
		cnt5m, cnt30m        int
		max5m, max30m        float64
		max2h, max5h, max10h float64
		sat1m, sat30m        int
		lastSat              time.Time
		hasLastSat           bool
	)

	for i := 0; i < h.size; i++ {
		idx := (h.head - 1 - i + maxHistorySamples) % maxHistorySamples
		s := h.buf[idx]
		if s.tsNano < c10h {
			break
		}
		v := s.value

		// Most-recent saturated sample encountered going backwards = last saturated event.
		if !hasLastSat && v >= SaturationThreshold {
			lastSat = time.Unix(0, s.tsNano)
			hasLastSat = true
		}

		if v > max10h {
			max10h = v
		}
		if s.tsNano >= c5h && v > max5h {
			max5h = v
		}
		if s.tsNano >= c2h && v > max2h {
			max2h = v
		}
		if s.tsNano >= c30m {
			sum30m += v
			cnt30m++
			if v > max30m {
				max30m = v
			}
			if v >= SaturationThreshold {
				sat30m++
				if s.tsNano >= c1m {
					sat1m++
				}
			}
		}
		if s.tsNano >= c5m {
			sum5m += v
			cnt5m++
			if v > max5m {
				max5m = v
			}
		}
	}

	var avg5m, avg30m float64
	if cnt5m > 0 {
		avg5m = sum5m / float64(cnt5m)
	}
	if cnt30m > 0 {
		avg30m = sum30m / float64(cnt30m)
	}

	return WindowStats{
		Avg5m:            avg5m,
		Max5m:            max5m,
		Avg30m:           avg30m,
		Max30m:           max30m,
		Max2h:            max2h,
		Max5h:            max5h,
		Max10h:           max10h,
		Saturated1m:      time.Duration(sat1m) * time.Second,
		Saturated30m:     time.Duration(sat30m) * time.Second,
		LastSaturatedAt:  lastSat,
		HasLastSaturated: hasLastSat,
	}
}
