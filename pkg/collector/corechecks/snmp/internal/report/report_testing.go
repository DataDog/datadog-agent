// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package report

const (
	fullIndex                   string = "9"
	ifSpeed                     uint64 = 80 * (1e6)
	mockTimeNowNano                    = int64(946684800000000000)
	mockTimeNowNano15SecEarlier        = int64(946684785000000000)
)

// MockInterfaceRateMap makes it easy to mock the map used for calculating state for bandwidth usage for testing
func MockInterfaceRateMap(interfaceID string, inIfSpeed uint64, outIfSpeed uint64, inSample float64, outSample float64, ts int64) InterfaceBandwidthState {
	irm := MakeInterfaceBandwidthState()
	irm[interfaceID+".ifBandwidthInUsage"] = &BandwidthUsage{
		ifSpeed:        inIfSpeed,
		previousSample: inSample,
		previousTsNano: ts,
	}
	irm[interfaceID+".ifBandwidthOutUsage"] = &BandwidthUsage{
		ifSpeed:        outIfSpeed,
		previousSample: outSample,
		previousTsNano: ts,
	}
	return irm
}

// Mock interface rate map with previous metric samples for the interface with ifSpeed of 30
func interfaceRateMapWithPrevious() InterfaceBandwidthState {
	return MockInterfaceRateMap(fullIndex, ifSpeed, ifSpeed, 30, 5, mockTimeNowNano15SecEarlier)
}

// Mock interface rate map with previous metric samples where the ifSpeed is taken from configuration files
func interfaceRateMapWithConfig() InterfaceBandwidthState {
	return MockInterfaceRateMap(fullIndex, 160_000_000, 40_000_000, 20, 10, mockTimeNowNano15SecEarlier)
}
