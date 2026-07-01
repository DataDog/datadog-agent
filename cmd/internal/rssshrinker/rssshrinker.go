// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package rssshrinker provides best-effort helpers to reduce process RSS.
package rssshrinker

import (
	"os"
	"strconv"
	"time"
)

const (
	// DisabledEnvVar disables the one-shot startup RSS shrinker when set to a truthy value.
	DisabledEnvVar = "DD_STARTUP_RSS_SHRINKER_DISABLED"
	// MallocTrimEnvVar enables an optional malloc_trim pass when set to a truthy value.
	MallocTrimEnvVar = "DD_STARTUP_RSS_SHRINKER_MALLOC_TRIM"
)

// DefaultStartupDelay is the delay before the startup RSS shrinker runs.
const DefaultStartupDelay = 2*time.Minute + 30*time.Second

func isEnvEnabled(name string) bool {
	value, found := os.LookupEnv(name)
	if !found {
		return false
	}

	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}

	return enabled
}
