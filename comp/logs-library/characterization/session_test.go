// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package characterization

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionBoundsAndAggregation(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	manager := NewManager()
	manager.now = func() time.Time { return now }

	started, err := manager.Start(10 * time.Second)
	require.NoError(t, err)
	require.Equal(t, "active", started.State)
	require.NotEmpty(t, started.SessionID)
	_, err = manager.Start(time.Second)
	require.ErrorIs(t, err, ErrSessionActive)

	observation := MessageObservation{ObservedAt: now.Add(time.Second), ContentBytes: 100, RawBytes: 120, TagCount: 2, TagBytes: 12, SourceType: "file", Pipeline: "0", PayloadFamily: "json", HasService: true, HasSourceID: true, SourceHash: 42}
	manager.Record(observation)
	observation.ObservedAt = now.Add(2 * time.Second)
	manager.Record(observation)
	observation.ObservedAt = now.Add(11 * time.Second)
	manager.Record(observation)

	result, err := manager.Stop(started.SessionID)
	require.NoError(t, err)
	require.Equal(t, "completed", result.State)
	require.Equal(t, uint64(2), result.Totals.Events)
	require.Equal(t, uint64(200), result.Totals.ContentBytes)
	require.Equal(t, uint64(1), result.Totals.SourceCount)
	require.Equal(t, uint64(1), result.Totals.Interarrivals.Count)
	require.Equal(t, uint64(2), result.PayloadFamilies["json"].Events)
	require.Len(t, result.Groups, 1)
}

func TestSessionAutomaticallyCompletesAndAllowsNext(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	manager := NewManager()
	manager.now = func() time.Time { return now }
	first, err := manager.Start(time.Second)
	require.NoError(t, err)
	now = now.Add(2 * time.Second)
	status, err := manager.Status()
	require.NoError(t, err)
	require.Equal(t, "completed", status.State)
	require.Equal(t, first.EndsAt, status.EndedAt)
	_, err = manager.Start(time.Second)
	require.NoError(t, err)
}

func TestSessionDoesNotRetainOrExposeSourceIdentifiers(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	manager := NewManager()
	manager.now = func() time.Time { return now }
	_, err := manager.Start(time.Second)
	require.NoError(t, err)
	manager.Record(MessageObservation{ObservedAt: now, SourceType: "file", Pipeline: "0", HasSourceID: true, SourceHash: 99})
	status, err := manager.Status()
	require.NoError(t, err)
	require.Equal(t, uint64(1), status.Totals.SourceCount)
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "99")
}

func TestDurationIsBounded(t *testing.T) {
	manager := NewManager()
	_, err := manager.Start(time.Second - 1)
	require.ErrorIs(t, err, ErrInvalidDuration)
	_, err = manager.Start(24*time.Hour + time.Second)
	require.ErrorIs(t, err, ErrInvalidDuration)
}
