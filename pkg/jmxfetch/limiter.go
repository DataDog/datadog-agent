package jmxfetch

import "time"

// restartLimiter checks if too many restarts happened within a time period
type restartLimiter struct {
	maxRestarts int
	interval    float64
	stopTimes   []time.Time
	idx         int
}

func newRestartLimiter(maxRestarts int, interval float64) restartLimiter {
	return restartLimiter{
		maxRestarts: maxRestarts,
		interval:    interval,
		stopTimes:   make([]time.Time, maxRestarts),
		idx:         0,
	}
}

func (r *restartLimiter) canRestart(now time.Time) bool {
	r.stopTimes[r.idx] = now
	oldestIdx := (r.idx + r.maxRestarts + 1) % r.maxRestarts

	// Please note that the zero value for `time.Time` is `0001-01-01 00:00:00 +0000 UTC`
	// therefore for the first iteration (the initial launch attempt), the interval will
	// always be biger than ival (jmx_restart_interval). In fact, this sub operation with
	// stopTimes here will only start yielding values potentially <= ival _after_ the first
	// maxRestarts attempts, which is fine and consistent.
	restartTooSoon := r.stopTimes[r.idx].Sub(r.stopTimes[oldestIdx]).Seconds() <= r.interval

	r.idx = (r.idx + 1) % r.maxRestarts

	return !restartTooSoon
}
