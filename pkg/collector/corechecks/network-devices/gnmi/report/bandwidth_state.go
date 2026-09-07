// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"errors"
	"fmt"
	"time"
)

// bandwidthTimeNow is only used when a cached sample carries no device timestamp.
var bandwidthTimeNow = time.Now

var errNoNewSample = errors.New("no new sample since the previous check run")

type bandwidthUsage struct {
	ifSpeed        uint64
	previousSample float64
	previousTsNano int64
	lastRate       float64
	hasRate        bool
}

// BandwidthState tracks per-series counter samples between check runs so that
// per-second rates can be derived from them.
type BandwidthState map[string]*bandwidthUsage

// NewBandwidthState returns an empty bandwidth utilization state map.
func NewBandwidthState() BandwidthState {
	return make(BandwidthState)
}

func (state BandwidthState) calculateUsageRate(interfaceID string, ifSpeed uint64, usageValue float64, sampleTime time.Time) (float64, error) {
	return state.counterRate(interfaceID, ifSpeed, usageValue, sampleTime)
}

// counterRate returns the per-second rate of a counter between its two most
// recent device samples.
//
// The interval is taken from the device sample timestamps, not from when the
// check runs: gNMI streams samples asynchronously, so a check run can read the
// same sample twice or skip one. Using the check's clock would then report 0
// followed by roughly double the real rate. When the sample has not changed
// since the previous run, the last computed rate is reported again.
//
// scale identifies the denominator the counter is normalised against (the
// interface speed for utilisation, 0 for raw counters); a change resets the
// series.
func (state BandwidthState) counterRate(seriesID string, scale uint64, value float64, sampleTime time.Time) (float64, error) {
	tsNano := sampleTime.UnixNano()
	if sampleTime.IsZero() {
		tsNano = bandwidthTimeNow().UnixNano()
	}

	entry, ok := state[seriesID]
	if !ok || entry.ifSpeed != scale {
		state[seriesID] = &bandwidthUsage{ifSpeed: scale, previousSample: value, previousTsNano: tsNano}
		if ok {
			return 0, fmt.Errorf("ifSpeed changed from %d to %d for interface ID %s, no rate emitted", entry.ifSpeed, scale, seriesID)
		}
		return 0, fmt.Errorf("new entry made, no rate emitted for interface ID %s", seriesID)
	}

	switch {
	case tsNano == entry.previousTsNano:
		if entry.hasRate {
			return entry.lastRate, nil
		}
		return 0, fmt.Errorf("%w for %s", errNoNewSample, seriesID)
	case tsNano < entry.previousTsNano:
		state[seriesID] = &bandwidthUsage{ifSpeed: scale, previousSample: value, previousTsNano: tsNano}
		return 0, fmt.Errorf("sample timestamp went backwards for %s, no rate emitted", seriesID)
	}

	elapsed := float64(tsNano-entry.previousTsNano) / float64(time.Second)
	delta := (value - entry.previousSample) / elapsed
	entry.previousSample = value
	entry.previousTsNano = tsNano

	if delta < 0 {
		entry.hasRate = false
		return 0, fmt.Errorf("rate value for interface ID %s is negative, discarding it", seriesID)
	}
	entry.lastRate, entry.hasRate = delta, true
	return delta, nil
}
