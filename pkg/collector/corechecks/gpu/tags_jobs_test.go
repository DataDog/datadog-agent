// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && linux && nvml

package gpu

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

var testJobsConfig = gpuconfig.JobsConfig{
	Run:   gpuconfig.IdentifierConfig{Key: "example/job-id-label", Type: gpuconfig.IdentifierTypeLabel},
	Group: gpuconfig.IdentifierConfig{Key: "example/job-group-annotation", Type: gpuconfig.IdentifierTypeAnnotation},
}

func TestJobTags(t *testing.T) {
	tests := []struct {
		name string
		meta workloadmeta.EntityMeta
		jobs gpuconfig.JobsConfig
		want []string
	}{
		{
			name: "label and annotation present",
			meta: workloadmeta.EntityMeta{
				Labels:      map[string]string{"example/job-id-label": "run-1"},
				Annotations: map[string]string{"example/job-group-annotation": "group-1"},
			},
			jobs: testJobsConfig,
			want: []string{"training_job_id:run-1", "training_group_id:group-1"},
		},
		{
			name: "type selects the source, label key in annotations is ignored",
			meta: workloadmeta.EntityMeta{
				Annotations: map[string]string{"example/job-id-label": "run-1"},
			},
			jobs: testJobsConfig,
			want: nil,
		},
		{
			name: "empty value is skipped",
			meta: workloadmeta.EntityMeta{
				Labels: map[string]string{"example/job-id-label": ""},
			},
			jobs: testJobsConfig,
			want: nil,
		},
		{
			name: "only run configured",
			meta: workloadmeta.EntityMeta{
				Labels:      map[string]string{"example/job-id-label": "run-1"},
				Annotations: map[string]string{"example/job-group-annotation": "group-1"},
			},
			jobs: gpuconfig.JobsConfig{Run: testJobsConfig.Run},
			want: []string{"training_job_id:run-1"},
		},
		{
			name: "unknown type is not configured",
			meta: workloadmeta.EntityMeta{
				Labels: map[string]string{"example/job-id-label": "run-1"},
			},
			jobs: gpuconfig.JobsConfig{Run: gpuconfig.IdentifierConfig{Key: "example/job-id-label", Type: "other"}},
			want: nil,
		},
		{
			name: "nothing configured",
			meta: workloadmeta.EntityMeta{
				Labels: map[string]string{"example/job-id-label": "run-1"},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jobTags(tt.meta, tt.jobs))
		})
	}
}

// setContainerInPod sets a container owned by a pod with the given metadata, as
// workloadmeta resolves a container's pod through the container's owner.
func setContainerInPod(mockWmeta workloadmetamock.Mock, containerID string, meta workloadmeta.EntityMeta) {
	podID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"}
	mockWmeta.Set(&workloadmeta.KubernetesPod{
		EntityID:   podID,
		EntityMeta: meta,
		Containers: []workloadmeta.OrchestratorContainer{{ID: containerID}},
	})
	mockWmeta.Set(&workloadmeta.Container{
		EntityID: newContainerWorkloadID(containerID),
		Runtime:  workloadmeta.ContainerRuntimeContainerd,
		Owner:    &podID,
	})
}

func TestBuildContainerTagsIncludesJobTags(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testJobsConfig)

	containerID := "test-container-id"
	workloadID := newContainerWorkloadID(containerID)
	setWorkloadTags(t, mocks.tagger, workloadID, nil, []string{"pod_name:trainer"}, nil)
	setContainerInPod(mocks.workloadMeta, containerID, workloadmeta.EntityMeta{
		Labels:      map[string]string{"example/job-id-label": "run-1"},
		Annotations: map[string]string{"example/job-group-annotation": "group-1"},
	})

	tags, err := cache.buildContainerTags(containerID)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pod_name:trainer", "training_job_id:run-1", "training_group_id:group-1"}, tags)
}

func TestBuildContainerTagsWithoutJobsConfigHasNoJobTags(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	containerID := "test-container-id"
	workloadID := newContainerWorkloadID(containerID)
	setWorkloadTags(t, mocks.tagger, workloadID, nil, []string{"pod_name:trainer"}, nil)
	setContainerInPod(mocks.workloadMeta, containerID, workloadmeta.EntityMeta{
		Labels: map[string]string{"example/job-id-label": "run-1"},
	})

	tags, err := cache.buildContainerTags(containerID)

	require.NoError(t, err)
	assert.Equal(t, []string{"pod_name:trainer"}, tags)
}

func TestBuildContainerTagsContainerWithoutPod(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testJobsConfig)

	containerID := "test-container-id"
	workloadID := newContainerWorkloadID(containerID)
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, workloadID, workloadmeta.ContainerRuntimeDocker)
	setWorkloadTags(t, mocks.tagger, workloadID, nil, nil, []string{"container_name:trainer"})

	tags, err := cache.buildContainerTags(containerID)

	require.NoError(t, err)
	assert.Equal(t, []string{"container_name:trainer"}, tags)
}

func TestBuildProcessTagsIncludesContainerJobTags(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testJobsConfig)

	pid := int32(1234)
	containerID := "container-123"
	containerEntityID := newContainerWorkloadID(containerID)
	setContainerInPod(mocks.workloadMeta, containerID, workloadmeta.EntityMeta{
		Labels:      map[string]string{"example/job-id-label": "run-1"},
		Annotations: map[string]string{"example/job-group-annotation": "group-1"},
	})
	mocks.workloadMeta.Set(&workloadmeta.Process{
		EntityID: newProcessWorkloadID(pid),
		NsPid:    pid,
		Owner:    &containerEntityID,
	})

	tags, err := cache.buildProcessTags(strconv.Itoa(int(pid)))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pid:1234", "nspid:1234", "training_job_id:run-1", "training_group_id:group-1"}, tags)
}
