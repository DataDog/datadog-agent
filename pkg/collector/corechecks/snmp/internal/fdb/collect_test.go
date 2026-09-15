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
)

func TestCollectQBridge(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)

	result := collect(sess, config{DeviceID: "default:1.2.3.4", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceQBridge, result.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3c", result.Entries[0].MacAddress)
	assert.Equal(t, int32(10), result.Entries[0].InterfaceIndex)
}

func TestCollectBridgeFallback(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := collect(sess, config{DeviceID: "ns:10.0.0.1", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceBridge, result.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3d", result.Entries[0].MacAddress)
	assert.Equal(t, int32(20), result.Entries[0].InterfaceIndex)
}

func TestCollectFiltersInvalidRows(t *testing.T) {
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
	// self(4) and mgmt(5) must not be treated as host locations.
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.80", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.80", 4)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.81", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.81", 5)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3c", result.Entries[0].MacAddress)
}

func TestCollectWithoutStatusTable(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, int32(10), result.Entries[0].InterfaceIndex)
}

func TestCollectTruncatedDiscardsRows(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	for i := 1; i <= 5; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50."+itoa(i), 1)
	}

	result := collect(sess, config{DeviceID: "d", MaxEntries: 2, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Equal(t, reasonMaxEntries, result.Reason)
	assert.Empty(t, result.Entries)
}

func TestCollectTruncatedQBridgeDoesNotFallBackToBridge(t *testing.T) {
	sess := session.CreateFakeSession()
	for i := 1; i <= 3; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50."+itoa(i), 1)
	}
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 1, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Empty(t, result.Entries)
	assert.NotEqual(t, SourceBridge, result.Source)
}

func TestCollectPortMapTruncationDiscardsRows(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 101)
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 102)
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.3", 103)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 3)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 1, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Equal(t, reasonMaxEntries, result.Reason)
	assert.Empty(t, result.Entries)
}

func TestCollectEmptySuccess(t *testing.T) {
	sess := session.CreateFakeSession()
	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Empty(t, result.Entries)
}

func TestCollectUsesFixedEntryLimit(t *testing.T) {
	sess := session.CreateFakeSession()
	for i := 0; i <= maxEntries; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30."+itoa(i/256)+"."+itoa(i%256), 1)
	}

	result := Collect(sess, "d", 10)
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Equal(t, reasonMaxEntries, result.Reason)
	assert.Empty(t, result.Entries)
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
