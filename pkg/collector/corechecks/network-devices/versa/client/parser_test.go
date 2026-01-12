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

func TestParsePathQoSMetrics(t *testing.T) {
	testData := [][]interface{}{
		{
			"test-branch-2B,test-branch-2C",
			"test-branch-2B",
			"test-branch-2C",
			1000.0,
			50.0,
			2000.0,
			25.0,
			1500.0,
			75.0,
			500.0,
			10.0,
			8000000.0,
			16000000.0,
			12000000.0,
			4000000.0,
			5000.0,
			160.0,
			3.2,
			40000000.0,
		},
	}

	expected := []QoSMetrics{
		{
			DrillKey:             "test-branch-2B,test-branch-2C",
			LocalSiteName:        "test-branch-2B",
			RemoteSiteName:       "test-branch-2C",
			BestEffortTx:         1000.0,
			BestEffortTxDrop:     50.0,
			ExpeditedForwardTx:   2000.0,
			ExpeditedForwardDrop: 25.0,
			AssuredForwardTx:     1500.0,
			AssuredForwardDrop:   75.0,
			NetworkControlTx:     500.0,
			NetworkControlDrop:   10.0,
			BestEffortBandwidth:  8000000.0,
			ExpeditedForwardBW:   16000000.0,
			AssuredForwardBW:     12000000.0,
			NetworkControlBW:     4000000.0,
			VolumeTx:             5000.0,
			TotalDrop:            160.0,
			PercentDrop:          3.2,
			Bandwidth:            40000000.0,
		},
	}

	result, err := parsePathQoSMetrics(testData)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestParseDIAMetrics(t *testing.T) {
	testData := [][]interface{}{
		{
			"test-branch-2B,DIA-1,192.168.1.1",
			"test-branch-2B",
			"DIA-1",
			"192.168.1.1",
			15000.0,
			12000.0,
			150000.0,
			120000.0,
		},
	}

	expected := []DIAMetrics{
		{
			DrillKey:      "test-branch-2B,DIA-1,192.168.1.1",
			Site:          "test-branch-2B",
			AccessCircuit: "DIA-1",
			IP:            "192.168.1.1",
			VolumeTx:      15000.0,
			VolumeRx:      12000.0,
			BandwidthTx:   150000.0,
			BandwidthRx:   120000.0,
		},
	}

	result, err := parseDIAMetrics(testData)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestParseSiteMetrics(t *testing.T) {
	testData := [][]interface{}{
		{
			"test-branch-2B",
			"123 Main St, Anytown, USA",
			"40.7128",
			"-74.0060",
			"GPS",
			15000.0,
			12000.0,
			150000.0,
			120000.0,
			99.5,
		},
	}

	expected := []SiteMetrics{
		{
			Site:           "test-branch-2B",
			Address:        "123 Main St, Anytown, USA",
			Latitude:       "40.7128",
			Longitude:      "-74.0060",
			LocationSource: "GPS",
			VolumeTx:       15000.0,
			VolumeRx:       12000.0,
			BandwidthTx:    150000.0,
			BandwidthRx:    120000.0,
			Availability:   99.5,
		},
	}

	result, err := parseSiteMetrics(testData)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestParseAnalyticsInterfaceMetrics(t *testing.T) {
	testData := [][]interface{}{
		{
			"test-branch-2B,INET-1,ge-0/0/1",
			"test-branch-2B",
			"INET-1",
			"ge-0/0/1",
			25.5,
			18.3,
			1024000.0,
			768000.0,
			1792000.0,
			8192.0,
			6144.0,
			14336.0,
		},
	}

	expected := []AnalyticsInterfaceMetrics{
		{
			DrillKey:    "test-branch-2B,INET-1,ge-0/0/1",
			Site:        "test-branch-2B",
			AccessCkt:   "INET-1",
			Interface:   "ge-0/0/1",
			RxUtil:      25.5,
			TxUtil:      18.3,
			VolumeRx:    1024000.0,
			VolumeTx:    768000.0,
			Volume:      1792000.0,
			BandwidthRx: 8192.0,
			BandwidthTx: 6144.0,
			Bandwidth:   14336.0,
		},
	}

	result, err := parseAnalyticsInterfaceMetrics(testData)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}
