// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import "time"

const (
	defaultDegradedDeadline = 0 // no deadline for degraded mode
)

func withinDegradedModePeriod(lastHeartbeat time.Time, degradedDuration time.Duration) bool {
	if degradedDuration == 0 {
		// infinite degraded period
		return true
	}

	return lastHeartbeat.Add(degradedDuration).After(time.Now())
}
