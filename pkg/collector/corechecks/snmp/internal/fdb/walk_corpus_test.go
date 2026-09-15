// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func collectFixture(t *testing.T, name string) Result {
	t.Helper()
	sess, err := loadWalkFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return Collect(sess, Config{
		DeviceID:           "ns:10.0.0.1",
		MaxEntries:         1000,
		MaxDuration:        time.Second,
		BulkMaxRepetitions: 10,
	})
}

func TestCollectAristaQBridgeWalk(t *testing.T) {
	result := collectFixture(t, "arista_eos.walk")
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	assert.Equal(t, metadata.FDBSourceQBridge, result.Status.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "4001", result.Entries[0].FDBID)
	assert.Equal(t, "00:09:0f:09:0a:09", result.Entries[0].MacAddress)
	assert.Equal(t, int32(105), result.Entries[0].BridgePort)
	assert.Equal(t, int32(1000014), result.Entries[0].InterfaceIndex)
	assert.Equal(t, "ns:10.0.0.1:1000014", result.Entries[0].InterfaceID)
}

func TestCollectCiscoCatalystBridgeFallbackWalk(t *testing.T) {
	result := collectFixture(t, "cisco_catalyst.walk")
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	assert.Equal(t, metadata.FDBSourceBridge, result.Status.Source)
	require.Len(t, result.Entries, 1)
	assert.Empty(t, result.Entries[0].FDBID)
	assert.Equal(t, "00:cc:fc:51:71:92", result.Entries[0].MacAddress)
	assert.Equal(t, int32(1), result.Entries[0].BridgePort)
	assert.Equal(t, int32(10101), result.Entries[0].InterfaceIndex)
	assert.Equal(t, "ns:10.0.0.1:10101", result.Entries[0].InterfaceID)
}

func TestCollectArubaPrefersQBridgeAndFiltersSelf(t *testing.T) {
	result := collectFixture(t, "aruba_jl074a.walk")
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	assert.Equal(t, metadata.FDBSourceQBridge, result.Status.Source)
	require.Len(t, result.Entries, 2)

	byMAC := map[string]metadata.FDBEntryMetadata{}
	for _, e := range result.Entries {
		byMAC[e.MacAddress] = e
		assert.Equal(t, metadata.FDBSourceQBridge, e.Source)
	}
	assert.Equal(t, int32(58), byMAC["00:07:32:3e:d2:1b"].BridgePort)
	assert.Equal(t, "ns:10.0.0.1:58", byMAC["00:07:32:3e:d2:1b"].InterfaceID)
	assert.Equal(t, int32(42), byMAC["00:60:b9:a8:64:6a"].BridgePort)
	_, self := byMAC["00:fd:45:77:83:80"]
	assert.False(t, self, "self/port-0 MAC should be dropped")
}

func TestCollectIfotecMultipleFDBIDsWithoutPortMap(t *testing.T) {
	result := collectFixture(t, "ifotec.snmprec")
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	require.Len(t, result.Entries, 3)

	ids := map[string]string{}
	for _, e := range result.Entries {
		ids[e.MacAddress] = e.FDBID
		assert.Zero(t, e.InterfaceIndex)
		assert.Empty(t, e.InterfaceID)
	}
	assert.Equal(t, "564", ids["60:31:97:6c:82:fc"])
	assert.Equal(t, "1003", ids["00:12:f1:13:01:a7"])
	assert.Equal(t, "3048", ids["d4:8c:b5:98:63:01"])
}

func TestCollectProcurveDropsMulticast(t *testing.T) {
	result := collectFixture(t, "procurve_2520.snmprec")
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "00:0a:f4:6b:37:9a", result.Entries[0].MacAddress)
	assert.Equal(t, "ns:10.0.0.1:28", result.Entries[0].InterfaceID)
}

func TestCollectJuniperLargeFDBIDDropsPortZero(t *testing.T) {
	result := collectFixture(t, "juniper_ex3400.walk")
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "196608", result.Entries[0].FDBID)
	assert.Equal(t, "00:10:db:ff:10:01", result.Entries[0].MacAddress)
	assert.Equal(t, int32(539), result.Entries[0].BridgePort)
	assert.Equal(t, int32(610), result.Entries[0].InterfaceIndex)
	assert.Equal(t, "ns:10.0.0.1:610", result.Entries[0].InterfaceID)
}

func TestCollectEdgeSwitchLengthPrefixedMAC(t *testing.T) {
	result := collectFixture(t, "edgeswitch_10xp.snmprec")
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "1", result.Entries[0].FDBID)
	assert.Equal(t, "00:0c:29:15:e6:1f", result.Entries[0].MacAddress)
	assert.Equal(t, int32(5), result.Entries[0].BridgePort)
	assert.Equal(t, "ns:10.0.0.1:5", result.Entries[0].InterfaceID)
}
