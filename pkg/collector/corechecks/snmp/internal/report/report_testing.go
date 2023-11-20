// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

// 15 seconds later
var mockTimeNowNano = int64(946684800000000000)
var mockTimeNowNano15SecLater = int64(946684785000000000)

// MockInterfaceRateMap makes it easy to mock the map used for calculating state for bandwidth usage for testing
func MockInterfaceRateMap(interfaceID string, inIfSpeed uint64, outIfSpeed uint64, inSample float64, outSample float64, ts int64) *InterfaceBandwidthState {
	irm := NewInterfaceBandwidthState()
	irm.state[interfaceID+".ifBandwidthInUsage"] = BandwidthUsage{
		ifSpeed:        inIfSpeed,
		previousSample: inSample,
		previousTsNano: ts,
	}
	irm.state[interfaceID+".ifBandwidthOutUsage"] = BandwidthUsage{
		ifSpeed:        outIfSpeed,
		previousSample: outSample,
		previousTsNano: ts,
	}
	return irm
}
