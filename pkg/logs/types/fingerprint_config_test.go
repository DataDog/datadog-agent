// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFingerprintConfigsEqualIncludesOpenFlags(t *testing.T) {
	base := &FingerprintConfig{
		FingerprintStrategy: FingerprintStrategyByteChecksum,
		Count:               8,
		OpenFlags:           []FileOpenFlag{FileOpenFlagDirect},
	}
	withoutDirect := &FingerprintConfig{
		FingerprintStrategy: FingerprintStrategyByteChecksum,
		Count:               8,
	}
	require.True(t, FingerprintConfigsEquivalent(base, withoutDirect))
	require.False(t, FingerprintConfigsEqual(base, withoutDirect))
}

func TestCloneFingerprintConfigCopiesOpenFlags(t *testing.T) {
	original := &FingerprintConfig{
		FingerprintStrategy: FingerprintStrategyLineChecksum,
		Count:               1,
		OpenFlags:           []FileOpenFlag{FileOpenFlagDirect},
	}
	clone := CloneFingerprintConfig(original)
	require.NotSame(t, original, clone)
	require.Equal(t, original.OpenFlags, clone.OpenFlags)
	clone.OpenFlags[0] = "mutated"
	require.Equal(t, []FileOpenFlag{FileOpenFlagDirect}, original.OpenFlags)
}
