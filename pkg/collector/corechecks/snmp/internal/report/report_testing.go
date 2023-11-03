// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

// MockInterfaceRateMap makes it easy to mock the map used for calculating rates for bandwidth usage for testing
func MockInterfaceRateMap(interfaceID string, inIfSpeed uint64, outIfSpeed uint64, inSample float64, outSample float64, ts float64) *InterfaceRateMap {
	irm := NewInterfaceRateMap()
	irm.rates[interfaceID+"ifBandwidthInUsage"] = InterfaceRate{
		ifSpeed:        inIfSpeed,
		previousSample: inSample,
		previousTs:     ts,
	}
	irm.rates[interfaceID+"ifBandwidthOutUsage"] = InterfaceRate{
		ifSpeed:        outIfSpeed,
		previousSample: outSample,
		previousTs:     ts,
	}
	return irm
}
