// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nccl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// In degraded mode (any of tagger/wmeta/telemetry is nil), the underlying
// WorkloadTagCache cannot be constructed, so ProcessTagger falls back to
// PID-only tagging. The tests below pin that contract.

func TestNewProcessTaggerDegradedMode(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil, nil)
	require.NotNil(t, pt)
	assert.Nil(t, pt.cache, "cache should be nil when components are missing")
}

func TestGetTagsForPIDDegradedMode(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil, nil)

	// Without cache, we still get the PID tag
	tags, err := pt.GetTagsForPID(12345)
	require.NoError(t, err)
	assert.Equal(t, []string{"pid:12345"}, tags)
}

func TestGetTagsForPIDZero(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil, nil)

	tags, err := pt.GetTagsForPID(0)
	require.NoError(t, err)
	assert.Equal(t, []string{"pid:0"}, tags)
}

func TestRefreshDegradedMode(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil, nil)

	// Refresh on a nil cache must be a no-op (not panic)
	require.NotPanics(t, func() { pt.Refresh() })
}

func TestSetContainerProviderDegradedMode(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil, nil)

	// SetContainerProvider on a nil cache must be a no-op (not panic)
	require.NotPanics(t, func() { pt.SetContainerProvider(nil) })
}
