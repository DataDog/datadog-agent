// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import "time"

func calculateCheckDelay(now time.Time, prevRunStats *Stats, execTime time.Duration) float64 {
	if prevRunStats.UpdateTimestamp.IsZero() || prevRunStats.Interval == 0 {
		return 0
	}

	previousCheckStartDate := prevRunStats.UpdateTimestamp.Add(-prevRunStats.LastExecutionTime)
	currentCheckStartDate := now.Add(-execTime)

	realInterval := currentCheckStartDate.Sub(previousCheckStartDate)
	delay := realInterval - prevRunStats.Interval

	return max(0, float64(delay)/float64(time.Second))
}
