// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePingSectionReadsTheBackendPayloadVerbatim(t *testing.T) {
	got, err := parsePingSection(json.RawMessage(`{
		"init_config": {"count": 2, "interval_ms": 10, "timeout_ms": 3000, "linux": {"use_raw_socket": true}},
		"instances": [
			{"ip_address": "10.0.0.2", "count": 5, "interval_ms": 100, "timeout_ms": 1000, "linux": {"use_raw_socket": false}},
			{"ip_address": "10.0.0.3"}
		]
	}`))
	require.NoError(t, err)

	assert.Equal(t, &pingOptions{
		Count:      2,
		IntervalMS: 10,
		TimeoutMS:  3000,
		Linux:      pingLinuxOptions{UseRawSocket: ptr(true)},
	}, got.shared())

	overrides, pinged := got.forDevice("10.0.0.2")
	require.True(t, pinged)
	assert.Equal(t, pingOptions{
		Count:      5,
		IntervalMS: 100,
		TimeoutMS:  1000,
		Linux:      pingLinuxOptions{UseRawSocket: ptr(false)},
	}, overrides)

	overrides, pinged = got.forDevice("10.0.0.3")
	require.True(t, pinged, "an instance with no overrides is still pinged")
	assert.Equal(t, pingOptions{}, overrides)

	_, pinged = got.forDevice("10.0.0.9")
	assert.False(t, pinged)
}

func TestParsePingSectionOfAnAbsentKeyPingsNothing(t *testing.T) {
	got, err := parsePingSection(nil)
	require.NoError(t, err)

	assert.Nil(t, got)
	assert.Nil(t, got.shared())
	_, pinged := got.forDevice("10.0.0.1")
	assert.False(t, pinged)
	assert.Empty(t, got.unmatched(map[string]struct{}{}))
}

func TestParsePingSectionWithNoInitConfigSharesNothing(t *testing.T) {
	got, err := parsePingSection(json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1"}]}`))
	require.NoError(t, err)

	assert.Nil(t, got.shared(), "the check keeps its own ping defaults")
	_, pinged := got.forDevice("10.0.0.1")
	assert.True(t, pinged)
}

func TestParsePingSectionRejectsAValueThatIsNotAPingKey(t *testing.T) {
	got, err := parsePingSection(json.RawMessage(`["not","an","object"]`))

	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestParsePingSectionSkipsAnInstanceWithNoIPAddress(t *testing.T) {
	got, err := parsePingSection(json.RawMessage(`{"instances":[{"count":3},{"ip_address":"10.0.0.1"}]}`))
	require.NoError(t, err)

	assert.Equal(t, []string{"10.0.0.1"}, got.orderedIPs)
}

func TestParsePingSectionKeepsTheFirstOfTwoInstancesSharingAnIPAddress(t *testing.T) {
	got, err := parsePingSection(json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.1","count":1},
		{"ip_address":"10.0.0.1","count":2}
	]}`))
	require.NoError(t, err)

	assert.Equal(t, []string{"10.0.0.1"}, got.orderedIPs)
	overrides, _ := got.forDevice("10.0.0.1")
	assert.Equal(t, 1, overrides.Count)
}

func TestUnmatchedReportsThePingedDevicesNoInstancePollsInDocumentOrder(t *testing.T) {
	got, err := parsePingSection(json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.9"},
		{"ip_address":"10.0.0.1"},
		{"ip_address":"192.168.1.1"}
	]}`))
	require.NoError(t, err)

	orphans := got.unmatched(map[string]struct{}{"10.0.0.1": {}})

	assert.Equal(t, []string{"10.0.0.9", "192.168.1.1"}, orphans)
}

func TestUnmatchedIsEmptyWhenEveryPingedDeviceIsPolled(t *testing.T) {
	got, err := parsePingSection(json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1"}]}`))
	require.NoError(t, err)

	assert.Empty(t, got.unmatched(map[string]struct{}{"10.0.0.1": {}}))
}
