// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package report is a package that reports metrics for the device check
package report

import (
	"fmt"
	"time"
)

// BandwidthUsage tracks the interface's current ifSpeed and last seen sample to generate rate with
type BandwidthUsage struct {
	ifSpeed        uint64
	previousSample float64
	previousTsNano int64
}

// InterfaceBandwidthState holds state between runs to be able to calculate rate
type InterfaceBandwidthState map[string]*BandwidthUsage

// MakeInterfaceBandwidthState returns a new InterfaceBandwidthState
func MakeInterfaceBandwidthState() InterfaceBandwidthState {
	return make(map[string]*BandwidthUsage)
}

// RemoveExpiredBandwidthUsageRates will go through the map of bandwidth usage rates and remove entries
// if it hasn't been updated in a given deadline (presumed to be an interface or device taken down if no metrics have been seen in the given amount of time)
func (ibs InterfaceBandwidthState) RemoveExpiredBandwidthUsageRates(timestampNano int64) {
	for k, v := range ibs {
		if v.previousTsNano <= timestampNano {
			delete(ibs, k)
		}
	}
}

/*
calculateBandwidthUsageRate is responsible for checking the state for previously seen metric sample to generate the rate from.
If the ifSpeed has changed for the interface, the rate will not be submitted (drop the previous sample)
*/
func (ibs InterfaceBandwidthState) calculateBandwidthUsageRate(fullIndex string, usageName string, ifSpeed uint64, usageValue float64) (float64, error) {
	interfaceID := fullIndex + "." + usageName
	state, ok := ibs[interfaceID]
	// current data point has the same interface speed as last data point
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

		if delta < 0 {
			return 0, fmt.Errorf("rate value for interface ID %s is negative, discarding it", interfaceID)
		}
		return delta, nil
	}
	// otherwise, no previous data point / different ifSpeed - make new entry for interface
	ibs[interfaceID] = &BandwidthUsage{
		ifSpeed:        ifSpeed,
		previousSample: usageValue,
		previousTsNano: TimeNow().UnixNano(),
	}
	// do not send a sample to metrics, send error for ifSpeed change (previous entry conflicted)
	if ok {
		return 0, fmt.Errorf("ifSpeed changed from %d to %d for interface ID %s, no rate emitted", state.ifSpeed, ifSpeed, interfaceID)
	}
	return 0, fmt.Errorf("new entry made, no rate emitted for interface ID %s", interfaceID)
}
