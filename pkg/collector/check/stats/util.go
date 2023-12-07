// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import "time"

func calculateCheckDelay(now time.Time, prevRunStats *Stats, execTime time.Duration) int64 {
	if prevRunStats.UpdateTimestamp == 0 || prevRunStats.Interval == 0 {
		return 0
	}

	previousCheckStartDate := prevRunStats.UpdateTimestamp - (prevRunStats.LastExecutionTime / 1e3)
	currentCheckStartDate := now.Unix() - int64(execTime.Seconds())

	delay := currentCheckStartDate - previousCheckStartDate - int64(prevRunStats.Interval.Seconds())

	// delay can be negative if a check recovers from delay
	if delay < 0 {
		delay = 0
	}

	return delay
}
