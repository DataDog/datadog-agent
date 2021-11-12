// +build !linux

package procutil

import "time"

// WithReturnZeroPermStats configures whether StatsWithPermByPID() returns StatsWithPerm that
// has zero values on all fields
func WithReturnZeroPermStats(enabled bool) Option {
	return func(p Probe) {}
}

// WithPermission configures if process collection should fetch fields
// that require elevated permission or not
func WithPermission(enabled bool) Option {
	return func(p Probe) {}
}

// WithBootTimeRefreshInterval configures the boot time refresh interval
func WithBootTimeRefreshInterval(bootTimeRefreshInterval time.Duration) Option {
	return func(p Probe) {}
}
