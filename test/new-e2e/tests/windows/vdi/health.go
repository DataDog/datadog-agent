// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package vdi

import "github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

const (
	statusOK       = 0
	statusCritical = 2
)

// healthBounds includes every matching report after the preceding phase,
// regardless of intake batching or delivery order.
func healthBounds(runs []*aggregator.CheckRun, status int, after int64) (first, latest int64) {
	for _, run := range runs {
		timestamp := int64(run.Timestamp)
		if run.Status != status || timestamp <= after {
			continue
		}
		if first == 0 || timestamp < first {
			first = timestamp
		}
		if timestamp > latest {
			latest = timestamp
		}
	}
	return first, latest
}
