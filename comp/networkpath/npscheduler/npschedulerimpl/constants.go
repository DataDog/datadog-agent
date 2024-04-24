package npschedulerimpl

import "time"

const (
	// DefaultFlushTickerInterval is the default flush interval in seconds
	DefaultFlushTickerInterval = 1 * time.Minute

	// DefaultPathtestRunDurationFromDiscovery is the default path test run duration
	DefaultPathtestRunDurationFromDiscovery = 10 * time.Minute

	// DefaultPathtestRunInterval is the default path test run duration
	DefaultPathtestRunInterval = 10 * time.Minute
)
