// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows || darwin

package logondurationimpl

import (
	"strings"
	"time"
)

const (
	clockSkewTolerance   = 5 * time.Second
	invalidTimelineTitle = "Device booted up: Boot timeline invalid"
)

// chainTimestamp is one timestamp of a timeline whose timestamps should be in chronological order.
type chainTimestamp struct {
	id    string
	value *time.Time
}

// missingInChain returns the IDs of unset timestamps in chain order.
func missingInChain(chain []chainTimestamp) []string {
	var missing []string
	for _, c := range chain {
		if c.value.IsZero() {
			missing = append(missing, c.id)
		}
	}
	return missing
}

// validateChain unsets impossible timestamps in place and returns their IDs in chain order.
// A timestamp is impossible when it is before boot, after now, or before the previous kept one,
// each with clockSkewTolerance of slack.
func validateChain(bootTime, now time.Time, chain []chainTimestamp) []string {
	dropped := make([]bool, len(chain))
	for i, c := range chain {
		if c.value.IsZero() {
			continue
		}
		if (!bootTime.IsZero() && c.value.Before(bootTime.Add(-clockSkewTolerance))) || c.value.After(now.Add(clockSkewTolerance)) {
			*c.value = time.Time{}
			dropped[i] = true
		}
	}

	var prev time.Time
	var invalid []string
	for i, c := range chain {
		if !c.value.IsZero() {
			if !prev.IsZero() && c.value.Before(prev.Add(-clockSkewTolerance)) {
				*c.value = time.Time{}
				dropped[i] = true
			} else {
				prev = *c.value
			}
		}
		if dropped[i] {
			invalid = append(invalid, c.id)
		}
	}
	return invalid
}

// addTimelineStatus records timeline_status and the missing and invalid timestamp IDs in custom.
func addTimelineStatus(custom map[string]interface{}, complete bool, missing, invalid []string) {
	switch {
	case len(invalid) > 0:
		custom["timeline_status"] = timelineInvalid
		custom["invalid_milestones"] = invalid
	case complete:
		custom["timeline_status"] = timelineComplete
	default:
		custom["timeline_status"] = timelineIncomplete
	}
	if len(missing) > 0 {
		custom["missing_milestones"] = missing
	}
}

func invalidTimelineMessage(invalid []string) string {
	return "Boot timeline invalid: timestamps outside this boot or out of order: " + strings.Join(invalid, ", ") + "."
}
