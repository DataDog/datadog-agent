// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"sync"
	"time"
)

// BandwidthUsage tracks the interface's current ifSpeed and last seen sample to generate rate with
type BandwidthUsage struct {
	ifSpeed        uint64
	previousSample float64
	previousTsNano int64
}

// InterfaceBandwidthState holds state between runs to be able to calculate rate and know if the ifSpeed has changed
type InterfaceBandwidthState struct {
	state map[string]BandwidthUsage
	mu    sync.RWMutex
}

// NewInterfaceBandwidthState creates a new InterfaceBandwidthState
func NewInterfaceBandwidthState() *InterfaceBandwidthState {
	return &InterfaceBandwidthState{state: make(map[string]BandwidthUsage)}
}

/*
calculateBandwidthUsageRate is responsible for checking the state for previously seen metric sample to generate the rate from.
If the ifSpeed has changed for the interface, the rate will not be submitted (drop the previous sample)
*/
func (ibs *InterfaceBandwidthState) calculateBandwidthUsageRate(deviceID string, fullIndex string, usageName string, ifSpeed uint64, usageValue float64) (float64, error) {
	interfaceID := deviceID + ":" + fullIndex + "." + usageName
	// current data point has the same interface speed as last data point
	ibs.mu.RLock()
	state, ok := ibs.state[interfaceID]
	ibs.mu.RUnlock()
	if ok && state.ifSpeed == ifSpeed {
		// Get time in seconds with nanosecond precision, as core agent uses for rate calculation in aggregator
		// https://github.com/DataDog/datadog-agent/blob/ecedf4648f41193988b4727fc6f893a0dfc3991e/pkg/aggregator/aggregator.go#L96
		currentTsNano := TimeNow().UnixNano()
		currentTs := float64(currentTsNano) / float64(time.Second)
		prevTs := float64(state.previousTsNano) / float64(time.Second)

		// calculate the delta, taken from pkg/metrics/rate.go
		// https://github.com/DataDog/datadog-agent/blob/ecedf4648f41193988b4727fc6f893a0dfc3991e/pkg/metrics/rate.go#L37
		delta := (usageValue - state.previousSample) / (currentTs - prevTs)

		// update the map previous as the current for next rate
		state.previousSample = usageValue
		state.previousTsNano = currentTsNano
		ibs.mu.Lock()
		ibs.state[interfaceID] = state
		ibs.mu.Unlock()

		if delta < 0 {
			return 0, fmt.Errorf("Rate value for device/interface %s is negative, discarding it", interfaceID)
		}
		return delta, nil
	}
	// otherwise, no previous data point / different ifSpeed - make new entry for interface
	ibs.mu.Lock()
	ibs.state[interfaceID] = BandwidthUsage{
		ifSpeed:        ifSpeed,
		previousSample: usageValue,
		previousTsNano: TimeNow().UnixNano(),
	}
	ibs.mu.Unlock()
	// do not send a sample to metrics, send error for ifSpeed change (previous entry conflicted)
	if ok {
		return 0, fmt.Errorf("ifSpeed changed from %d to %d for device and interface %s, no rate emitted", state.ifSpeed, ifSpeed, interfaceID)
	}
	return 0, nil
}
