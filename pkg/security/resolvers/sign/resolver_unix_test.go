// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package sign

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func createProcessContext(cgroupID string, pid uint32) *model.ProcessContext {
	pce := &model.ProcessContext{}
	pce.Process.Pid = pid
	pce.Process.CGroup.CGroupID = containerutils.CGroupID(cgroupID)
	return pce
}

func TestSign_ReturnsConsistentSignature(t *testing.T) {
	resolver := &Resolver{signatureKey: 12345}
	pce := createProcessContext("cgroup-123", 1000)

	sig1, err := resolver.Sign(pce)
	require.NoError(t, err)

	sig2, err := resolver.Sign(pce)
	require.NoError(t, err)

	assert.Equal(t, sig1, sig2, "same inputs should produce the same signature")
}

func TestSign_DifferentCgroupID_DifferentSignature(t *testing.T) {
	resolver := &Resolver{signatureKey: 12345}

	pce1 := createProcessContext("cgroup-123", 1000)
	pce2 := createProcessContext("cgroup-456", 1000)

	sig1, err := resolver.Sign(pce1)
	require.NoError(t, err)

	sig2, err := resolver.Sign(pce2)
	require.NoError(t, err)

	assert.NotEqual(t, sig1, sig2, "different cgroupID should produce different signatures")
}

func TestSign_DifferentKey_DifferentSignature(t *testing.T) {
	resolver1 := &Resolver{signatureKey: 12345}
	resolver2 := &Resolver{signatureKey: 67890}

	pce := createProcessContext("cgroup-123", 1000)

	sig1, err := resolver1.Sign(pce)
	require.NoError(t, err)

	sig2, err := resolver2.Sign(pce)
	require.NoError(t, err)

	assert.NotEqual(t, sig1, sig2, "different signatureKey should produce different signatures")
}

func TestSign_SignatureIsValidHex(t *testing.T) {
	resolver := &Resolver{signatureKey: 12345}
	pce := createProcessContext("cgroup-123", 1000)

	sig, err := resolver.Sign(pce)
	require.NoError(t, err)

	// SHA256 produces 64 hex characters
	assert.Len(t, sig, 64, "signature should be 64 characters (sha256 hex)")

	// Verify it's valid hex
	for _, c := range sig {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"signature should only contain hex characters, got: %c", c)
	}
}

func TestSign_EmptyCgroupID_ProducesDifferentSignature(t *testing.T) {
	resolver := &Resolver{signatureKey: 12345}

	pce1 := createProcessContext("", 1000)
	pce2 := createProcessContext("cgroup-123", 1000)

	sig1, err := resolver.Sign(pce1)
	require.NoError(t, err)

	sig2, err := resolver.Sign(pce2)
	require.NoError(t, err)

	assert.NotEqual(t, sig1, sig2, "empty cgroupID should produce different signature than non-empty")
}
