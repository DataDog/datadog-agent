// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package traininginfo

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	testGPU0 = "GPU-00000000-0000-0000-0000-000000000000"
	testGPU1 = "GPU-11111111-1111-1111-1111-111111111111"
)

func rayRunJobs() gpuconfig.JobsConfig {
	return gpuconfig.JobsConfig{
		Run: gpuconfig.IdentifierConfig{Key: "_RAY_SUBMISSION_ID", Type: gpuconfig.IdentifierTypeEnv},
	}
}

func listProcesses(processes ...GPUProcess) ListGPUProcessesFunc {
	return func() ([]GPUProcess, error) {
		return processes, nil
	}
}

func TestCollect(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 10, Env: map[string]string{"_RAY_SUBMISSION_ID": "raysubmit_1", "SECRET": "hunter2"}},
		{Pid: 20, Env: map[string]string{"_RAY_SUBMISSION_ID": "raysubmit_2"}},
		{Pid: 30, Env: map[string]string{"OTHER": "value"}},
	})

	handler := NewHandler(rayRunJobs(), nil, procRoot, listProcesses(
		GPUProcess{PID: 10, DeviceUUID: testGPU0},
		GPUProcess{PID: 10, DeviceUUID: testGPU1},
		GPUProcess{PID: 20, DeviceUUID: testGPU1},
		GPUProcess{PID: 30, DeviceUUID: testGPU1}, // no identifier set
		GPUProcess{PID: 40, DeviceUUID: testGPU1}, // exited process
	))

	assert.ElementsMatch(t, []model.TrainingInfo{
		{PID: 10, DeviceUUID: testGPU0, TrainingRunID: "raysubmit_1"},
		{PID: 10, DeviceUUID: testGPU1, TrainingRunID: "raysubmit_1"},
		{PID: 20, DeviceUUID: testGPU1, TrainingRunID: "raysubmit_2"},
	}, handler.Collect())
}

func TestCollectIgnoresDisallowedEnvVars(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 10, Env: map[string]string{"_RAY_SUBMISSION_ID": "raysubmit_1", "SECRET": "hunter2"}},
	})

	jobs := rayRunJobs()
	jobs.Group = gpuconfig.IdentifierConfig{Key: "SECRET", Type: gpuconfig.IdentifierTypeEnv}
	handler := NewHandler(jobs, nil, procRoot, listProcesses(GPUProcess{PID: 10, DeviceUUID: testGPU0}))

	assert.Equal(t, []model.TrainingInfo{
		{PID: 10, DeviceUUID: testGPU0, TrainingRunID: "raysubmit_1"},
	}, handler.Collect())
}

func TestCollectRayRunAndGroup(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 10, Env: map[string]string{"_RAY_SUBMISSION_ID": "raysubmit_1", "RAY_CLUSTER_NAME": "cluster"}},
	})

	jobs := rayRunJobs()
	jobs.Group = gpuconfig.IdentifierConfig{Key: "RAY_CLUSTER_NAME", Type: gpuconfig.IdentifierTypeEnv}
	handler := NewHandler(jobs, nil, procRoot, listProcesses(GPUProcess{PID: 10, DeviceUUID: testGPU0}))

	assert.Equal(t, []model.TrainingInfo{
		{PID: 10, DeviceUUID: testGPU0, TrainingRunID: "raysubmit_1", TrainingGroupID: "cluster"},
	}, handler.Collect())
}

func TestCollectMLflowRunAndGroup(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 10, Env: map[string]string{"MLFLOW_RUN_ID": "run-1", "MLFLOW_EXPERIMENT_ID": "42"}},
	})

	jobs := gpuconfig.JobsConfig{
		Run:   gpuconfig.IdentifierConfig{Key: "MLFLOW_RUN_ID", Type: gpuconfig.IdentifierTypeEnv},
		Group: gpuconfig.IdentifierConfig{Key: "MLFLOW_EXPERIMENT_ID", Type: gpuconfig.IdentifierTypeEnv},
	}
	handler := NewHandler(jobs, nil, procRoot, listProcesses(GPUProcess{PID: 10, DeviceUUID: testGPU0}))

	assert.Equal(t, []model.TrainingInfo{
		{PID: 10, DeviceUUID: testGPU0, TrainingRunID: "run-1", TrainingGroupID: "42"},
	}, handler.Collect())
}

func TestCollectExtraAllowedEnvVars(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 10, Env: map[string]string{"JOB_ID": "job-1", "EXPERIMENT": "exp-1"}},
	})

	jobs := gpuconfig.JobsConfig{
		Run:   gpuconfig.IdentifierConfig{Key: "JOB_ID", Type: gpuconfig.IdentifierTypeEnv},
		Group: gpuconfig.IdentifierConfig{Key: "EXPERIMENT", Type: gpuconfig.IdentifierTypeEnv},
	}
	handler := NewHandler(jobs, []string{"JOB_ID"}, procRoot, listProcesses(GPUProcess{PID: 10, DeviceUUID: testGPU0}))

	assert.Equal(t, []model.TrainingInfo{
		{PID: 10, DeviceUUID: testGPU0, TrainingRunID: "job-1"},
	}, handler.Collect())
}

func TestCollectDisabled(t *testing.T) {
	tests := []struct {
		name string
		jobs gpuconfig.JobsConfig
	}{
		{"not configured", gpuconfig.JobsConfig{}},
		{"label and annotation", gpuconfig.JobsConfig{
			Run:   gpuconfig.IdentifierConfig{Key: "_RAY_SUBMISSION_ID", Type: gpuconfig.IdentifierTypeLabel},
			Group: gpuconfig.IdentifierConfig{Key: "_RAY_SUBMISSION_ID", Type: gpuconfig.IdentifierTypeAnnotation},
		}},
		{"disallowed env var", gpuconfig.JobsConfig{
			Run: gpuconfig.IdentifierConfig{Key: "SECRET", Type: gpuconfig.IdentifierTypeEnv},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(tt.jobs, nil, t.TempDir(), func() ([]GPUProcess, error) {
				require.Fail(t, "GPU processes should not be listed when disabled")
				return nil, nil
			})

			assert.False(t, handler.Enabled())
			assert.Empty(t, handler.Collect())
		})
	}
}

func TestCollectPartialListing(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 10, Env: map[string]string{"_RAY_SUBMISSION_ID": "raysubmit_1"}},
	})

	handler := NewHandler(rayRunJobs(), nil, procRoot, func() ([]GPUProcess, error) {
		return []GPUProcess{{PID: 10, DeviceUUID: testGPU0}}, errors.New("device lost")
	})

	assert.Equal(t, []model.TrainingInfo{
		{PID: 10, DeviceUUID: testGPU0, TrainingRunID: "raysubmit_1"},
	}, handler.Collect())
}

func TestHandleTrainingInfo(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 10, Env: map[string]string{"_RAY_SUBMISSION_ID": "raysubmit_1"}},
	})
	handler := NewHandler(rayRunJobs(), nil, procRoot, listProcesses(GPUProcess{PID: 10, DeviceUUID: testGPU0}))

	recorder := httptest.NewRecorder()
	handler.HandleTrainingInfo(recorder, httptest.NewRequest(http.MethodGet, "/training-info", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `[{"pid":10,"device_uuid":"`+testGPU0+`","training_run_id":"raysubmit_1"}]`, recorder.Body.String())
}
