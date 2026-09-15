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
	return collect(sess, config{
		DeviceID:           "ns:10.0.0.1",
		MaxEntries:         1000,
		MaxDuration:        time.Second,
		BulkMaxRepetitions: 10,
	})
}

func TestCollectAristaQBridgeWalk(t *testing.T) {
	result := collectFixture(t, "arista_eos.walk")
	require.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceQBridge, result.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "00:09:0f:09:0a:09", result.Entries[0].MacAddress)
	assert.Equal(t, int32(1000014), result.Entries[0].InterfaceIndex)
}

func TestCollectCiscoCatalystBridgeFallbackWalk(t *testing.T) {
	result := collectFixture(t, "cisco_catalyst.walk")
	require.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceBridge, result.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "00:cc:fc:51:71:92", result.Entries[0].MacAddress)
	assert.Equal(t, int32(10101), result.Entries[0].InterfaceIndex)
}

func TestCollectArubaPrefersQBridgeAndFiltersSelf(t *testing.T) {
	result := collectFixture(t, "aruba_jl074a.walk")
	require.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceQBridge, result.Source)
	require.Len(t, result.Entries, 2)

	byMAC := map[string]metadata.FDBEntryMetadata{}
	for _, e := range result.Entries {
		byMAC[e.MacAddress] = e
	}
	assert.Equal(t, int32(58), byMAC["00:07:32:3e:d2:1b"].InterfaceIndex)
	assert.Equal(t, int32(42), byMAC["00:60:b9:a8:64:6a"].InterfaceIndex)
	_, self := byMAC["00:fd:45:77:83:80"]
	assert.False(t, self, "self/port-0 MAC should be dropped")
}

func TestCollectIfotecWithoutPortMapDropsEntries(t *testing.T) {
	result := collectFixture(t, "ifotec.snmprec")
	require.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceQBridge, result.Source)
	assert.Empty(t, result.Entries)
}

func TestCollectProcurveDropsMulticast(t *testing.T) {
	result := collectFixture(t, "procurve_2520.snmprec")
	require.Equal(t, OutcomeSuccess, result.Outcome)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "00:0a:f4:6b:37:9a", result.Entries[0].MacAddress)
	assert.Equal(t, int32(28), result.Entries[0].InterfaceIndex)
}

func TestCollectJuniperDropsPortZero(t *testing.T) {
	result := collectFixture(t, "juniper_ex3400.walk")
	require.Equal(t, OutcomeSuccess, result.Outcome)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "00:10:db:ff:10:01", result.Entries[0].MacAddress)
	assert.Equal(t, int32(610), result.Entries[0].InterfaceIndex)
}

func TestCollectEdgeSwitchLengthPrefixedMAC(t *testing.T) {
	result := collectFixture(t, "edgeswitch_10xp.snmprec")
	require.Equal(t, OutcomeSuccess, result.Outcome)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "00:0c:29:15:e6:1f", result.Entries[0].MacAddress)
	assert.Equal(t, int32(5), result.Entries[0].InterfaceIndex)
}
