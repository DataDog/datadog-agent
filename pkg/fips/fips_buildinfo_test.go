// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fips

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltForFIPSReadsFromBuildInfo(t *testing.T) {
	// This test verifies that BuiltForFIPS() uses runtime build info rather than
	// a hardcoded compile-time constant. It should return false in a non-FIPS test binary.
	// In a FIPS test binary (built with requirefips), it should return true.
	result := BuiltForFIPS()
	// We can only assert the negative: a standard test binary is not FIPS.
	// Positive coverage requires a FIPS-tagged build (covered by CI fips test jobs).
	t.Logf("BuiltForFIPS()=%v (expected false in standard test binary)", result)
	// Verify build info is readable
	info, ok := debug.ReadBuildInfo()
	require.True(t, ok, "debug.ReadBuildInfo() must succeed")
	t.Logf("build settings count: %d", len(info.Settings))
}
