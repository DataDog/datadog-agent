// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package discovery

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
)

// fakeComponent records the ranges it is handed and reports scripted errors.
type fakeComponent struct {
	errs map[string]error // range id -> error to report

	calls [][]ndmdiscovery.Range
}

func (c *fakeComponent) Schedule(ranges []ndmdiscovery.Range) map[string]error {
	c.calls = append(c.calls, ranges)
	return c.errs
}

func (c *fakeComponent) RangeCount() int { return 0 }

func newTestHandler(t *testing.T, comp *fakeComponent) *Handler {
	t.Helper()
	return NewHandler(comp, logmock.New(t))
}

const oneRange = `{"ranges":[{
	"autodiscovery_id":"ad-1",
	"namespace":"prod",
	"cidr":"10.0.0.0/24",
	"credential_ids":["cred-a"],
	"interval_sec":900,
	"ignored_ip_addresses":["10.0.0.1"],
	"tags":["site:paris"],
	"snmp_options":{"port":1161,"timeout_ms":2000,"retries":0},
	"ping_options":{"count":2,"interval_ms":500,"timeout_ms":1000}
}]}`

func TestKeyIsDiscovery(t *testing.T) {
	assert.Equal(t, "discovery", newTestHandler(t, &fakeComponent{}).Key())
}

func TestSnapshotPassesTheWholePayloadThrough(t *testing.T) {
	comp := &fakeComponent{}

	errs := newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{
		"path-a": json.RawMessage(oneRange),
	})

	assert.Empty(t, errs)
	require.Len(t, comp.calls, 1)
	require.Len(t, comp.calls[0], 1)

	r := comp.calls[0][0]
	assert.Equal(t, "ad-1", r.ID)
	assert.Equal(t, "prod", r.Namespace)
	assert.Equal(t, "10.0.0.0/24", r.CIDR)
	assert.Equal(t, []string{"cred-a"}, r.CredentialIDs)
	assert.Equal(t, 900, r.IntervalSec)
	assert.Equal(t, []string{"10.0.0.1"}, r.IgnoredIPAddresses)
	assert.Equal(t, []string{"site:paris"}, r.Tags)

	require.NotNil(t, r.SNMPOptions)
	assert.Equal(t, 1161, r.SNMPOptions.Port)
	assert.Equal(t, 2000, r.SNMPOptions.TimeoutMs)
	require.NotNil(t, r.SNMPOptions.Retries)
	assert.Equal(t, 0, *r.SNMPOptions.Retries, "an explicit zero is not a missing value")

	require.NotNil(t, r.PingOptions)
	assert.Equal(t, 2, r.PingOptions.Count)
	assert.Equal(t, 500, r.PingOptions.IntervalMs)
	assert.Equal(t, 1000, r.PingOptions.TimeoutMs)
}

func TestSnapshotLeavesAbsentOptionSectionsNil(t *testing.T) {
	comp := &fakeComponent{}

	newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{
		"path-a": json.RawMessage(`{"ranges":[{"autodiscovery_id":"ad-1","cidr":"10.0.0.0/24","credential_ids":["cred-a"]}]}`),
	})

	require.Len(t, comp.calls[0], 1)
	assert.Nil(t, comp.calls[0][0].SNMPOptions)
	assert.Nil(t, comp.calls[0][0].PingOptions)
}

func TestSnapshotMergesTheRangesOfEveryPath(t *testing.T) {
	comp := &fakeComponent{}

	errs := newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{
		"path-a": json.RawMessage(`{"ranges":[{"autodiscovery_id":"ad-1","cidr":"10.0.0.0/24","credential_ids":["cred-a"]}]}`),
		"path-b": json.RawMessage(`{"ranges":[{"autodiscovery_id":"ad-2","cidr":"10.0.1.0/24","credential_ids":["cred-a"]}]}`),
	})

	assert.Empty(t, errs)
	require.Len(t, comp.calls, 1)
	assert.Len(t, comp.calls[0], 2)
}

func TestSnapshotSchedulesNothingForAnEmptySnapshot(t *testing.T) {
	comp := &fakeComponent{}

	errs := newTestHandler(t, comp).Snapshot(nil)

	assert.Empty(t, errs)
	require.Len(t, comp.calls, 1, "the component is still told, so it stops what it was sweeping")
	assert.Empty(t, comp.calls[0])
}

func TestSnapshotRejectsARangeIDClaimedByTwoPaths(t *testing.T) {
	comp := &fakeComponent{}
	body := json.RawMessage(`{"ranges":[{"autodiscovery_id":"ad-1","cidr":"10.0.0.0/24","credential_ids":["cred-a"]}]}`)

	errs := newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{"path-a": body, "path-b": body})

	require.Len(t, errs, 2, "both claimants are told")
	assert.Contains(t, errs["path-a"].Error(), "ad-1")
	assert.Contains(t, errs["path-b"].Error(), "ad-1")
	assert.Len(t, comp.calls[0], 1, "the first claim still runs")
}

func TestSnapshotRejectsAMalformedPathAndKeepsTheOthers(t *testing.T) {
	comp := &fakeComponent{}

	errs := newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{
		"path-a": json.RawMessage(`[]`),
		"path-b": json.RawMessage(`{"ranges":[{"autodiscovery_id":"ad-2","cidr":"10.0.1.0/24","credential_ids":["cred-a"]}]}`),
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs["path-a"].Error(), "not an object")
	assert.Len(t, comp.calls[0], 1)
}

func TestSnapshotRejectsARangeWithNoID(t *testing.T) {
	comp := &fakeComponent{}

	errs := newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{
		"path-a": json.RawMessage(`{"ranges":[{"cidr":"10.0.0.0/24"}]}`),
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs["path-a"].Error(), "autodiscovery_id")
	assert.Empty(t, comp.calls[0])
}

func TestSnapshotMapsAComponentRejectionBackToItsPath(t *testing.T) {
	comp := &fakeComponent{errs: map[string]error{"ad-2": errors.New("cidr is required")}}

	errs := newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{
		"path-a": json.RawMessage(`{"ranges":[{"autodiscovery_id":"ad-1","cidr":"10.0.0.0/24","credential_ids":["cred-a"]}]}`),
		"path-b": json.RawMessage(`{"ranges":[{"autodiscovery_id":"ad-2","credential_ids":["cred-a"]}]}`),
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs["path-b"].Error(), "ad-2")
	assert.Contains(t, errs["path-b"].Error(), "cidr is required")
}

func TestSnapshotReportsEveryRejectedRangeOfOnePath(t *testing.T) {
	comp := &fakeComponent{errs: map[string]error{
		"ad-1": errors.New("cidr is required"),
		"ad-2": errors.New("credential_ids must hold at least one credential"),
	}}

	errs := newTestHandler(t, comp).Snapshot(map[string]json.RawMessage{
		"path-a": json.RawMessage(`{"ranges":[
			{"autodiscovery_id":"ad-1","credential_ids":["cred-a"]},
			{"autodiscovery_id":"ad-2","cidr":"10.0.1.0/24"}
		]}`),
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs["path-a"].Error(), "ad-1")
	assert.Contains(t, errs["path-a"].Error(), "ad-2")
}
