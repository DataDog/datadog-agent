// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package logondurationimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logonduration"
)

// loginChain returns the OSLog timestamps of ts in boot order.
func loginChain(ts *logonduration.LoginTimestamps) []chainTimestamp {
	return []chainTimestamp{
		{id: "login_window_ready", value: &ts.LoginWindowTime},
		{id: "login", value: &ts.LoginTime},
		{id: "desktop_ready", value: &ts.DesktopReadyTime},
	}
}

// missingMilestones returns the IDs of unset timestamps in chain order.
func missingMilestones(ts logonduration.LoginTimestamps) []string {
	return missingInChain(loginChain(&ts))
}

// validateTimestamps drops impossible timestamps and returns their milestone IDs in chain order.
func validateTimestamps(bootTime, now time.Time, ts logonduration.LoginTimestamps) (logonduration.LoginTimestamps, []string) {
	invalid := validateChain(bootTime, now, loginChain(&ts))
	return ts, invalid
}
