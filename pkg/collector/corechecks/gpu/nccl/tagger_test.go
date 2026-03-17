// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nccl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestNewProcessTaggerNilComponents(t *testing.T) {
	// Test that NewProcessTagger works with nil components
	pt := NewProcessTagger(nil, nil, nil)
	require.NotNil(t, pt)
	assert.Nil(t, pt.tagger)
	assert.Nil(t, pt.wmeta)
	assert.Nil(t, pt.containerProvider)
}

func TestGetTagsForPIDWithoutProvider(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	// Without any components, should still return PID tag
	tags, err := pt.GetTagsForPID(12345)
	require.NoError(t, err)
	assert.Contains(t, tags, "pid:12345")
}

func TestGetTagsForPIDZero(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	// PID 0 should still return tags
	tags, err := pt.GetTagsForPID(0)
	require.NoError(t, err)
	assert.Contains(t, tags, "pid:0")
}

func TestRefresh(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	// Set up some cached data
	pt.pidToCid = map[int]string{123: "container1"}

	// Refresh should clear the cache
	pt.Refresh()
	assert.Nil(t, pt.pidToCid)
}

func TestGetWorkloadTagsProcess(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   "12345",
	}

	tags, err := pt.GetWorkloadTags(workloadID)
	require.NoError(t, err)
	assert.Contains(t, tags, "pid:12345")
}

func TestGetWorkloadTagsInvalidProcessID(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   "not-a-number",
	}

	_, err := pt.GetWorkloadTags(workloadID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid process ID")
}

func TestGetWorkloadTagsUnsupportedKind(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "some-pod",
	}

	_, err := pt.GetWorkloadTags(workloadID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported workload kind")
}

func TestGetTagsForPIDWithGPU(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	tags, err := pt.GetTagsForPIDWithGPU(12345, "GPU-abc123")
	require.NoError(t, err)
	assert.Contains(t, tags, "pid:12345")
	assert.Contains(t, tags, "gpu_uuid:GPU-abc123")
}

func TestGetTagsForPIDWithGPUEmpty(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	tags, err := pt.GetTagsForPIDWithGPU(12345, "")
	require.NoError(t, err)
	assert.Contains(t, tags, "pid:12345")
	// Should not contain gpu_uuid tag when empty
	for _, tag := range tags {
		assert.NotContains(t, tag, "gpu_uuid:")
	}
}

func TestGetWorkloadTagsForPID(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	tags, err := pt.GetWorkloadTagsForPID(54321)
	require.NoError(t, err)
	assert.Contains(t, tags, "pid:54321")
}

func TestGetContainerIDWithoutProvider(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	containerID, err := pt.getContainerID(12345)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no container provider available")
	assert.Empty(t, containerID)
}

func TestGetWorkloadTagsContainer(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil)

	// Container workload without wmeta set will fail
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   "container-123",
	}

	// Should fail because wmeta is nil
	_, err := pt.GetWorkloadTags(workloadID)
	assert.Error(t, err)
}
