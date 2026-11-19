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

const (
	clockSkewTolerance   = 5 * time.Second
	invalidTimelineTitle = "Device booted up: Boot timeline invalid"
)

// validateTimestamps drops impossible timestamps and returns their milestone IDs in chain order.
func validateTimestamps(bootTime, now time.Time, ts logonduration.LoginTimestamps) (logonduration.LoginTimestamps, []string) {
	milestones := []struct {
		id      string
		value   *time.Time
		dropped bool
	}{
		{id: "login_window_ready", value: &ts.LoginWindowTime},
		{id: "login", value: &ts.LoginTime},
		{id: "desktop_ready", value: &ts.DesktopReadyTime},
	}

	for i := range milestones {
		m := &milestones[i]
		if m.value.IsZero() {
			continue
		}
		if (!bootTime.IsZero() && m.value.Before(bootTime.Add(-clockSkewTolerance))) || m.value.After(now.Add(clockSkewTolerance)) {
			*m.value = time.Time{}
			m.dropped = true
		}
	}

	var prev time.Time
	var dropped []string
	for i := range milestones {
		m := &milestones[i]
		if !m.value.IsZero() {
			if !prev.IsZero() && m.value.Before(prev.Add(-clockSkewTolerance)) {
				*m.value = time.Time{}
				m.dropped = true
			} else {
				prev = *m.value
			}
		}
		if m.dropped {
			dropped = append(dropped, m.id)
		}
	}
	return ts, dropped
}
