// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package eventstream

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NewTestMonitor creates a minimal Monitor suitable for testing.
// It initializes only the maps required to avoid nil pointer panics
// when CountEvent and CountInvalidEvent are called.
func NewTestMonitor() *Monitor {
	// Initialize invalidEventStats with entries for the EventStreamMap
	var invalidStats [maxInvalidEventCause]*GenericStats
	for i := 0; i < int(maxInvalidEventCause); i++ {
		stats := makeGenericStats()
		invalidStats[i] = &stats
	}

	return &Monitor{
		eventStats: make(map[string][][model.MaxKernelEventType]EventStats),
		invalidEventStats: map[string][maxInvalidEventCause]*GenericStats{
			EventStreamMap: invalidStats,
		},
	}
}
