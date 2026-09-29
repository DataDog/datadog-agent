// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && linux && nvml

package gpu

import (
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

var testJobsConfig = gpuconfig.JobsConfig{
	Run:   gpuconfig.IdentifierConfig{Key: "example/job-id-label", Type: gpuconfig.IdentifierTypeLabel},
	Group: gpuconfig.IdentifierConfig{Key: "example/job-group-annotation", Type: gpuconfig.IdentifierTypeAnnotation},
}

var testEnvJobsConfig = gpuconfig.JobsConfig{
	Run:   gpuconfig.IdentifierConfig{Key: "JOB_ID", Type: gpuconfig.IdentifierTypeEnv},
	Group: gpuconfig.IdentifierConfig{Key: "JOB_GROUP", Type: gpuconfig.IdentifierTypeEnv},
}

func TestJobTags(t *testing.T) {
	tests := []struct {
		name string
		meta workloadmeta.EntityMeta
		env  model.ProcessJobIDs
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
			name: "env vars",
			env:  model.ProcessJobIDs{Run: "run-1", Group: "group-1"},
			jobs: testEnvJobsConfig,
			want: []string{"training_job_id:run-1", "training_group_id:group-1"},
		},
		{
			name: "env var missing or empty",
			env:  model.ProcessJobIDs{Group: ""},
			jobs: testEnvJobsConfig,
			want: nil,
		},
		{
			name: "env type ignores labels with the same key",
			meta: workloadmeta.EntityMeta{Labels: map[string]string{"JOB_ID": "run-1"}},
			jobs: testEnvJobsConfig,
			want: nil,
		},
		{
			name: "mixed sources",
			meta: workloadmeta.EntityMeta{Labels: map[string]string{"example/job-id-label": "run-1"}},
			env:  model.ProcessJobIDs{Group: "group-1"},
			jobs: gpuconfig.JobsConfig{Run: testJobsConfig.Run, Group: testEnvJobsConfig.Group},
			want: []string{"training_job_id:run-1", "training_group_id:group-1"},
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
			assert.Equal(t, tt.want, jobTags(tt.meta, tt.env, tt.jobs))
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

func TestBuildContainerTagsIncludesEnvJobTags(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testEnvJobsConfig)

	var gotPIDs []int
	cache.readJobIDs = func(pid int) (model.ProcessJobIDs, error) {
		gotPIDs = append(gotPIDs, pid)
		return model.ProcessJobIDs{Run: "run-1", Group: "group-1"}, nil
	}

	containerID := "test-container-id"
	workloadID := newContainerWorkloadID(containerID)
	mocks.workloadMeta.Set(&workloadmeta.Container{
		EntityID: workloadID,
		Runtime:  workloadmeta.ContainerRuntimeContainerd,
		PID:      4242,
	})

	tags, err := cache.buildContainerTags(containerID)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"training_job_id:run-1", "training_group_id:group-1"}, tags)
	assert.Equal(t, []int{4242}, gotPIDs, "the identifiers of a process are read in a single call")
}

func TestBuildContainerTagsEnvJobTagsReadErrorOrNoPID(t *testing.T) {
	tests := []struct {
		name string
		pid  int
		read JobIDReader
	}{
		{"read error", 4242, func(int) (model.ProcessJobIDs, error) { return model.ProcessJobIDs{}, errors.New("permission denied") }},
		{"no pid", 0, func(int) (model.ProcessJobIDs, error) { return model.ProcessJobIDs{Run: "should-not-be-read"}, nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, mocks := setupWorkloadTagCache(t)
			cache.SetJobsConfig(testEnvJobsConfig)
			cache.readJobIDs = tt.read

			containerID := "test-container-id"
			mocks.workloadMeta.Set(&workloadmeta.Container{
				EntityID: newContainerWorkloadID(containerID),
				Runtime:  workloadmeta.ContainerRuntimeContainerd,
				PID:      tt.pid,
			})

			tags, err := cache.buildContainerTags(containerID)

			require.NoError(t, err)
			assert.Empty(t, tags)
		})
	}
}

func TestOverrideTags(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		overrides []string
		want      []string
	}{
		{"no overrides", []string{"a:1"}, nil, []string{"a:1"}},
		{"adds new tag", []string{"a:1"}, []string{"b:2"}, []string{"a:1", "b:2"}},
		{"replaces same name", []string{"a:1", "b:old", "c:3"}, []string{"b:new"}, []string{"a:1", "c:3", "b:new"}},
		{"replaces only exact name", []string{"training_job_id_x:1"}, []string{"training_job_id:2"}, []string{"training_job_id_x:1", "training_job_id:2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := slices.Clone(tt.tags)
			assert.Equal(t, tt.want, overrideTags(tt.tags, tt.overrides))
			assert.Equal(t, original, tt.tags, "input must not be modified")
		})
	}
}

// setJobIDsByPID makes the cache read the job identifiers from the given per-PID values.
func setJobIDsByPID(cache *WorkloadTagCache, idsByPID map[int]model.ProcessJobIDs) {
	cache.readJobIDs = func(pid int) (model.ProcessJobIDs, error) {
		return idsByPID[pid], nil
	}
}

func TestBuildProcessTagsJobTagsFromProcessEnv(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testEnvJobsConfig)

	pid := int32(1234)
	containerID := "container-123"
	containerEntityID := newContainerWorkloadID(containerID)
	mocks.workloadMeta.Set(&workloadmeta.Container{EntityID: containerEntityID, Runtime: workloadmeta.ContainerRuntimeContainerd, PID: 100})
	mocks.workloadMeta.Set(&workloadmeta.Process{
		EntityID: newProcessWorkloadID(pid),
		NsPid:    pid,
		Owner:    &containerEntityID,
	})
	setJobIDsByPID(cache, map[int]model.ProcessJobIDs{
		// container init process only has the run ID, the GPU process has both
		100:      {Run: "container-run"},
		int(pid): {Run: "process-run", Group: "process-group"},
	})

	tags, err := cache.buildProcessTags(strconv.Itoa(int(pid)))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"pid:1234", "nspid:1234",
		"training_job_id:process-run", "training_group_id:process-group",
	}, tags)
}

func TestBuildProcessTagsJobTagsFallBackToContainerEnv(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testEnvJobsConfig)

	pid := int32(1234)
	containerEntityID := newContainerWorkloadID("container-123")
	mocks.workloadMeta.Set(&workloadmeta.Container{EntityID: containerEntityID, Runtime: workloadmeta.ContainerRuntimeContainerd, PID: 100})
	mocks.workloadMeta.Set(&workloadmeta.Process{
		EntityID: newProcessWorkloadID(pid),
		NsPid:    pid,
		Owner:    &containerEntityID,
	})
	setJobIDsByPID(cache, map[int]model.ProcessJobIDs{
		100: {Run: "container-run"},
	})

	tags, err := cache.buildProcessTags(strconv.Itoa(int(pid)))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pid:1234", "nspid:1234", "training_job_id:container-run"}, tags)
}

func TestBuildProcessTagsJobTagsProcessWithoutContainer(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testEnvJobsConfig)

	pid := int32(1234)
	mocks.workloadMeta.Set(&workloadmeta.Process{EntityID: newProcessWorkloadID(pid), NsPid: pid})
	mocks.containerProvider.EXPECT().GetPidToCid(time.Duration(0)).Return(map[int]string{})
	setJobIDsByPID(cache, map[int]model.ProcessJobIDs{
		int(pid): {Run: "process-run"},
	})

	tags, err := cache.buildProcessTags(strconv.Itoa(int(pid)))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pid:1234", "nspid:1234", "training_job_id:process-run"}, tags)
}

func TestBuildProcessTagsJobTagsNoEnvIdentifierDoesNotReadEnv(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	cache.SetJobsConfig(testJobsConfig) // label + annotation only

	pid := int32(1234)
	mocks.workloadMeta.Set(&workloadmeta.Process{EntityID: newProcessWorkloadID(pid), NsPid: pid})
	mocks.containerProvider.EXPECT().GetPidToCid(time.Duration(0)).Return(map[int]string{})
	cache.readJobIDs = func(int) (model.ProcessJobIDs, error) {
		require.Fail(t, "the environment must not be read when no env identifier is configured")
		return model.ProcessJobIDs{}, nil
	}

	tags, err := cache.buildProcessTags(strconv.Itoa(int(pid)))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pid:1234", "nspid:1234"}, tags)
}
