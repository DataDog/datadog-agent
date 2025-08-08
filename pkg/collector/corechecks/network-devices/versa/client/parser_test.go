// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLinkUsageMetrics(t *testing.T) {
	testData := [][]interface{}{
		{
			"test-branch-2B,INET-1",
			"test-branch-2B",
			"INET-1",
			"10000000000",
			"10000000000",
			"Unknown",
			"Unknown",
			"10.20.20.7",
			"",
			757144.0,
			457032.0,
			6730.168888888889,
			4062.5066666666667,
		},
	}

	expected := []LinkUsageMetrics{
		{
			DrillKey:          "test-branch-2B,INET-1",
			Site:              "test-branch-2B",
			AccessCircuit:     "INET-1",
			UplinkBandwidth:   "10000000000",
			DownlinkBandwidth: "10000000000",
			Type:              "Unknown",
			Media:             "Unknown",
			IP:                "10.20.20.7",
			ISP:               "",
			VolumeTx:          757144.0,
			VolumeRx:          457032.0,
			BandwidthTx:       6730.168888888889,
			BandwidthRx:       4062.5066666666667,
		},
	}

	result, err := parseLinkUsageMetrics(testData)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestParseLinkStatusMetrics(t *testing.T) {
	testData := [][]interface{}{
		{
			"test-branch-2B,INET-1",
			"test-branch-2B",
			"INET-1",
			98.5,
		},
	}

	expected := []LinkStatusMetrics{
		{
			DrillKey:      "test-branch-2B,INET-1",
			Site:          "test-branch-2B",
			AccessCircuit: "INET-1",
			Availability:  98.5,
		},
	}

	result, err := parseLinkStatusMetrics(testData)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestParseTunnelMetrics(t *testing.T) {
	testData := [][]interface{}{
		{
			"test-branch-2B,10.1.1.1",
			"test-branch-2B",
			"10.1.1.1",
			"10.2.2.2",
			"vpn-profile-1",
			67890.0,
			12345.0,
		},
	}

	expected := []TunnelMetrics{
		{
			DrillKey:    "test-branch-2B,10.1.1.1",
			Appliance:   "test-branch-2B",
			LocalIP:     "10.1.1.1",
			RemoteIP:    "10.2.2.2",
			VpnProfName: "vpn-profile-1",
			VolumeRx:    67890.0,
			VolumeTx:    12345.0,
		},
	}

	result, err := parseTunnelMetrics(testData)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}
