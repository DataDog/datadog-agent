// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rssshrinker

import "time"

// ScheduleDefault schedules a single best-effort RSS shrink pass after DefaultStartupDelay.
func ScheduleDefault() {
	Schedule(DefaultStartupDelay)
}

// Schedule schedules a single best-effort RSS shrink pass after delay.
func Schedule(delay time.Duration) {
	if !canSchedule() || isEnvEnabled(DisabledEnvVar) {
		return
	}

	time.AfterFunc(delay, Shrink)
}
