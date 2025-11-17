// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import "time"

// FlushConfig contains the configuration for how the NetFlow collector should flush flows to the EP Forwarder.
type FlushConfig struct {
	// how long a flow should aggregate for before being sent to EP Forwarder
	FlowCollectionDuration time.Duration
	// interval for checking flows to flush and send them to EP Forwarder
	FlushTickFrequency time.Duration
}

// FlushContext contains the context for a flush operation. It contains simple information about the flush, including
// a consistent timestamp that can be used.
type FlushContext struct {
	FlushTime     time.Time
	LastFlushedAt time.Time
	// NumFlushes is the # of ticks that are included in this flush. In rare cases, the time.Ticker may
	// skip ticks, so we'll end up flushing a couple ticks worth of events.
	NumFlushes int64
}
