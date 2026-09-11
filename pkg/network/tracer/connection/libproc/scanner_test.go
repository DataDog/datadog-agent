// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package libproc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLibprocLimitsRejectUnboundedScans(t *testing.T) {
	for _, limits := range []Limits{
		{},
		{MaxPIDs: 1, MaxFDsPerPID: 1},
		{MaxPIDs: 1, MaxObservations: 1},
		{MaxFDsPerPID: 1, MaxObservations: 1},
	} {
		require.Error(t, limits.validate())
	}
	require.NoError(t, DefaultLimits.validate())
}
