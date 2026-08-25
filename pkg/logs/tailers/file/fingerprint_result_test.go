// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func TestFingerprintsMatchUnderSameConfig(t *testing.T) {
	cfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               1024,
	}
	other := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               2048,
	}
	a := &types.Fingerprint{Value: 42, Config: cfg}
	b := &types.Fingerprint{Value: 42, Config: cfg}
	c := &types.Fingerprint{Value: 42, Config: other}

	assert.True(t, types.FingerprintsMatchUnderSameConfig(a, b))
	assert.False(t, types.FingerprintsMatchUnderSameConfig(a, c))
}

func TestHasAuthoritativeDirectCandidate(t *testing.T) {
	directCfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               1024,
		OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
	}
	fp := &types.Fingerprint{Value: 99, Config: directCfg}

	assert.True(t, HasAuthoritativeDirectCandidate(directCfg, FingerprintResult{
		Fingerprint:  fp,
		AppliedFlags: []types.FileOpenFlag{types.FileOpenFlagDirect},
	}))
	assert.False(t, HasAuthoritativeDirectCandidate(directCfg, FingerprintResult{
		Fingerprint:  fp,
		AppliedFlags: nil,
	}))
}
