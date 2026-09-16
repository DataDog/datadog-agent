// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package normalize

import (
	"math"
	"time"
)

// Year2000NanosecTS is an arbitrary cutoff to spot weird-looking start timestamps.
var Year2000NanosecTS = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()

// FixDuration validates a span duration relative to its start time, returning a
// corrected value and whether the input was invalid. A negative duration, or one
// that would overflow when added to start, is reset to 0.
func FixDuration(start, duration int64) (fixed int64, invalid bool) {
	if duration < 0 {
		return 0, true
	}
	if duration > math.MaxInt64-start {
		return 0, true
	}
	return duration, false
}

// FixStartTime validates a span start time, returning a corrected value and
// whether the input was invalid. A start predating Year2000NanosecTS is
// considered garbage and reset to "now - duration" (clamped to "now").
func FixStartTime(start, duration int64) (fixed int64, invalid bool) {
	if start < Year2000NanosecTS {
		now := time.Now().UnixNano()
		newStart := now - duration
		if newStart < 0 {
			return now, true
		}
		return newStart, true
	}
	return start, false
}
