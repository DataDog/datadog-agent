// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"fmt"
	"time"
)

var bandwidthTimeNow = time.Now

type bandwidthUsage struct {
	ifSpeed        uint64
	previousSample float64
	previousTsNano int64
}

// BandwidthState tracks per-interface utilization samples between check runs.
type BandwidthState map[string]*bandwidthUsage

// NewBandwidthState returns an empty bandwidth utilization state map.
func NewBandwidthState() BandwidthState {
	return make(BandwidthState)
}

func (state BandwidthState) calculateUsageRate(interfaceID string, ifSpeed uint64, usageValue float64) (float64, error) {
	entry, ok := state[interfaceID]
	if ok && entry.ifSpeed == ifSpeed {
		currentTsNano := bandwidthTimeNow().UnixNano()
		currentTs := float64(currentTsNano) / float64(time.Second)
		prevTs := float64(entry.previousTsNano) / float64(time.Second)

		delta := (usageValue - entry.previousSample) / (currentTs - prevTs)
		entry.previousSample = usageValue
		entry.previousTsNano = currentTsNano

		if delta < 0 {
			return 0, fmt.Errorf("rate value for interface ID %s is negative, discarding it", interfaceID)
		}
		return delta, nil
	}

	state[interfaceID] = &bandwidthUsage{
		ifSpeed:        ifSpeed,
		previousSample: usageValue,
		previousTsNano: bandwidthTimeNow().UnixNano(),
	}
	if ok {
		return 0, fmt.Errorf("ifSpeed changed from %d to %d for interface ID %s, no rate emitted", entry.ifSpeed, ifSpeed, interfaceID)
	}
	return 0, fmt.Errorf("new entry made, no rate emitted for interface ID %s", interfaceID)
}
