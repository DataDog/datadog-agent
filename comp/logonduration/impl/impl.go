// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package logondurationimpl implements the logon duration component
package logondurationimpl

import (
	logonduration "github.com/DataDog/datadog-agent/comp/logonduration/def"
)

// persistentCacheKey stores the last boot time to detect reboots across agent restarts.
const persistentCacheKey = "logon_duration:last_boot_time"

// Provides defines what this component provides
type Provides struct {
	Comp logonduration.Component
}

// Milestone represents a single event in the boot/logon timeline.
type Milestone struct {
	Name      string  `json:"name"`
	OffsetS   float64 `json:"offset_s"`
	DurationS float64 `json:"duration_s"`
	Timestamp string  `json:"timestamp"`
}
