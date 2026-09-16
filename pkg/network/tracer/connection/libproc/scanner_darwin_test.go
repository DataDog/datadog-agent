// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && cgo

package libproc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeScannerHostBufferStartsBelowMaxObservations(t *testing.T) {
	scanner, err := NewNativeScanner(DefaultLimits)
	require.NoError(t, err)
	require.Equal(t, initialHostObservations, len(scanner.hostRaw))
	require.Less(t, len(scanner.hostRaw), DefaultLimits.MaxObservations)
}

func TestNativeScannerGrowHostRawStopsAtMaxObservations(t *testing.T) {
	scanner, err := NewNativeScanner(Limits{MaxPIDs: 1, MaxFDsPerPID: 1, MaxObservations: initialHostObservations + 1})
	require.NoError(t, err)
	require.Equal(t, initialHostObservations, len(scanner.hostRaw))
	scanner.growHostRaw()
	require.Equal(t, initialHostObservations+1, len(scanner.hostRaw))
	scanner.growHostRaw()
	require.Equal(t, initialHostObservations+1, len(scanner.hostRaw))
}

func TestHostObservationSeedRespectsMax(t *testing.T) {
	require.Equal(t, 8, hostObservationSeed(8))
	require.Equal(t, initialHostObservations, hostObservationSeed(DefaultLimits.MaxObservations))
}
