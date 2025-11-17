package common

import "time"

type FlushConfig struct {
	// how long a flow should aggregate for before being sent to EP Forwarder
	FlowCollectionDuration time.Duration
	// interval for checking flows to flush and send them to EP Forwarder
	FlushTickFrequency time.Duration
}

type FlushContext struct {
	FlushTime     time.Time
	LastFlushedAt time.Time
	// NumFlushes is the # of ticks that are included in this flush. In rare cases, the time.Ticker may
	// skip ticks, so we'll end up flushing a couple ticks worth of events.
	NumFlushes int64
}
