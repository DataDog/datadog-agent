package npschedulerimpl

import "time"

const (
	// DefaultFlushTickerInterval is the default flush interval in seconds
	DefaultFlushTickerInterval = 10 * time.Second

	// DefaultPathtestRunDurationFromDiscovery is the default path test run duration
	DefaultPathtestRunDurationFromDiscovery = 10 * time.Minute

	// DefaultPathtestRunInterval is the default path test run duration
	DefaultPathtestRunInterval = 1 * time.Minute
)
