// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package vdi

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

func TestHealthBounds(t *testing.T) {
	// The first failed poll at 100 must be checked even when it arrives in
	// the same batch as later failures, after startup and healthy reports.
	runs := []*aggregator.CheckRun{
		{Status: statusCritical, Timestamp: 130},
		{Status: statusCritical, Timestamp: 80},
		{Status: statusOK, Timestamp: 90},
		{Status: statusCritical, Timestamp: 90},
		{Status: statusCritical, Timestamp: 100},
		{Status: statusOK, Timestamp: 140},
		{Status: statusCritical, Timestamp: 115},
	}
	first, latest := healthBounds(runs, statusCritical, 90)
	require.Equal(t, int64(100), first)
	require.Equal(t, int64(130), latest)

	// A delayed earlier report must move the start boundary backwards.
	runs = append(runs, &aggregator.CheckRun{Status: statusCritical, Timestamp: 95})
	first, latest = healthBounds(runs, statusCritical, 90)
	require.Equal(t, int64(95), first)
	require.Equal(t, int64(130), latest)

	first, latest = healthBounds(runs, statusOK, 130)
	require.Equal(t, int64(140), first)
	require.Equal(t, int64(140), latest)

	first, latest = healthBounds(runs, statusCritical, 130)
	require.Zero(t, first)
	require.Zero(t, latest)

	first, latest = healthBounds(nil, statusCritical, 90)
	require.Zero(t, first)
	require.Zero(t, latest)
}
