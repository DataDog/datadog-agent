// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestCollectQBridge(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)

	result := Collect(sess, Config{DeviceID: "default:1.2.3.4", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	assert.Equal(t, metadata.FDBSourceQBridge, result.Status.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3c", result.Entries[0].MacAddress)
	assert.Equal(t, int32(1), result.Entries[0].BridgePort)
	assert.Equal(t, int32(10), result.Entries[0].InterfaceIndex)
	assert.Equal(t, "default:1.2.3.4:10", result.Entries[0].InterfaceID)
	assert.Equal(t, "1", result.Entries[0].FDBID)
}

func TestCollectBridgeFallback(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := Collect(sess, Config{DeviceID: "ns:10.0.0.1", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	assert.Equal(t, metadata.FDBSourceBridge, result.Status.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3d", result.Entries[0].MacAddress)
	assert.Equal(t, int32(2), result.Entries[0].BridgePort)
	assert.Equal(t, "ns:10.0.0.1:20", result.Entries[0].InterfaceID)
	assert.Empty(t, result.Entries[0].FDBID)
}

func TestCollectFilters(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	// learned unicast
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)
	// multicast
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.1.0.94.0.0.1", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.1.0.94.0.0.1", 3)
	// broadcast
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.255.255.255.255.255.255", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.255.255.255.255.255.255", 3)
	// zero MAC
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.0.0.0.0.0.0", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.0.0.0.0.0.0", 3)
	// zero port
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.70", 0)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.70", 3)
	// self (not learned)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.80", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.80", 4)
	// mgmt (not learned)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.81", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.81", 5)

	result := Collect(sess, Config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3c", result.Entries[0].MacAddress)
}

func TestCollectMissingStatusKept(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)

	result := Collect(sess, Config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, int32(1), result.Entries[0].BridgePort)
}

func TestCollectTruncatedDiscardsRows(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	for i := 1; i <= 5; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50."+itoa(i), 1)
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50."+itoa(i), 3)
	}

	result := Collect(sess, Config{DeviceID: "d", MaxEntries: 2, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, metadata.FDBCollectStatusTruncated, result.Status.Status)
	assert.Equal(t, reasonMaxEntries, result.Status.Reason)
	assert.Empty(t, result.Entries)
}

func TestCollectTruncatedQBridgeDoesNotFallBackToBridge(t *testing.T) {
	sess := session.CreateFakeSession()
	for i := 1; i <= 3; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50."+itoa(i), 1)
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50."+itoa(i), 3)
	}
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := Collect(sess, Config{DeviceID: "d", MaxEntries: 1, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, metadata.FDBCollectStatusTruncated, result.Status.Status)
	assert.Empty(t, result.Entries)
	assert.NotEqual(t, metadata.FDBSourceBridge, result.Status.Source)
}

func TestCollectPortMapTruncationOmitsUnmappedInterfaceID(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 101)
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 102)
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.3", 103)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 3)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)

	result := Collect(sess, Config{DeviceID: "d", MaxEntries: 1, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	require.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, int32(3), result.Entries[0].BridgePort)
	assert.Zero(t, result.Entries[0].InterfaceIndex)
	assert.Empty(t, result.Entries[0].InterfaceID)
}

func TestCollectEmptySuccess(t *testing.T) {
	sess := session.CreateFakeSession()
	result := Collect(sess, Config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, metadata.FDBCollectStatusSuccess, result.Status.Status)
	assert.Empty(t, result.Entries)
	assert.Equal(t, 0, result.Status.RowCount)
}

func TestWalkDeadline(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	res := walkColumn(sess, oidDot1qTpFdbPort, 10, 100, time.Now().Add(-time.Second))
	assert.Equal(t, reasonMaxDuration, res.reason)
	assert.Error(t, res.err)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
