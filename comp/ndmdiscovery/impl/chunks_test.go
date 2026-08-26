// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChunkPlanSlash24(t *testing.T) {
	p, err := newChunkPlan("10.0.0.0/24", nil, 65536)
	require.NoError(t, err)

	assert.Equal(t, 1, p.chunkCount())
	assert.Equal(t, 256, p.totalAddresses())
	assert.Equal(t, 0, p.ignoredCount())

	c := p.chunk(0)
	assert.Equal(t, 0, c.Index)
	require.Len(t, c.Targets, 256)
	// Network and broadcast addresses are included, matching the legacy listener.
	assert.Equal(t, "10.0.0.0", c.Targets[0])
	assert.Equal(t, "10.0.0.255", c.Targets[255])
}

func TestNewChunkPlanSlash16IsChunkedAndLazy(t *testing.T) {
	p, err := newChunkPlan("10.0.0.0/16", nil, 65536)
	require.NoError(t, err)

	assert.Equal(t, 256, p.chunkCount())
	assert.Equal(t, 65536, p.totalAddresses())

	first := p.chunk(0)
	assert.Equal(t, "10.0.0.0", first.Targets[0])

	last := p.chunk(255)
	assert.Equal(t, 255, last.Index)
	require.Len(t, last.Targets, 256)
	assert.Equal(t, "10.0.255.0", last.Targets[0])
	assert.Equal(t, "10.0.255.255", last.Targets[255])
}

func TestNewChunkPlanPartialFinalChunk(t *testing.T) {
	p, err := newChunkPlan("10.0.0.0/22", nil, 65536)
	require.NoError(t, err)
	assert.Equal(t, 4, p.chunkCount())
	assert.Equal(t, 1024, p.totalAddresses())

	// /25 is smaller than one chunk.
	p2, err := newChunkPlan("10.0.0.0/25", nil, 65536)
	require.NoError(t, err)
	assert.Equal(t, 1, p2.chunkCount())
	assert.Equal(t, 128, p2.totalAddresses())
	assert.Len(t, p2.chunk(0).Targets, 128)
}

func TestChunkPlanExcludesIgnoredAddresses(t *testing.T) {
	p, err := newChunkPlan("10.0.0.0/24", []string{"10.0.0.1", "10.0.0.2"}, 65536)
	require.NoError(t, err)

	assert.Equal(t, 2, p.ignoredCount())
	assert.Equal(t, 256, p.totalAddresses())

	targets := p.chunk(0).Targets
	assert.Len(t, targets, 254)
	assert.NotContains(t, targets, "10.0.0.1")
	assert.NotContains(t, targets, "10.0.0.2")
	assert.Contains(t, targets, "10.0.0.3")
}

func TestChunkPlanIgnoresAddressesOutsideRange(t *testing.T) {
	p, err := newChunkPlan("10.0.0.0/24", []string{"192.168.1.1"}, 65536)
	require.NoError(t, err)
	// Outside the range, so it does not count against progress.
	assert.Equal(t, 0, p.ignoredCount())
	assert.Len(t, p.chunk(0).Targets, 256)
}

func TestNewChunkPlanRejectsTooLargeRange(t *testing.T) {
	_, err := newChunkPlan("10.0.0.0/12", nil, 65536)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum")
}

func TestNewChunkPlanRejectsInvalidInput(t *testing.T) {
	_, err := newChunkPlan("not-a-cidr", nil, 65536)
	require.Error(t, err)

	_, err = newChunkPlan("2001:db8::/64", nil, 65536)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IPv4")

	_, err = newChunkPlan("10.0.0.5/24", nil, 65536)
	require.NoError(t, err, "a host bit set in the CIDR is masked, not rejected")
}
