// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package integrationtests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gpuBurnerBinEnv             = "GPU_BURNER_BIN"
	gpuBurnerStartupLimit       = 90 * time.Second
	gpuBurnerRunTime            = 30
	gpuBurnerCalibrationSeconds = 10
)

// GPUBurnerMetrics is the live GPU metric snapshot returned by gpu-burner.
type GPUBurnerMetrics struct {
	SMActive float64 `json:"sm_active"`
}

// GPUBurnerWorker describes a gpu-burner worker returned by its status API.
type GPUBurnerWorker struct {
	GPUUUID string            `json:"gpu_uuid"`
	Stage   string            `json:"stage"`
	Metrics *GPUBurnerMetrics `json:"metrics"`
}

// GPUBurnerStatus is the response returned by the gpu-burner status API.
type GPUBurnerStatus struct {
	Stage   string            `json:"stage"`
	Workers []GPUBurnerWorker `json:"workers"`
}

// GPUBurner is a gpu-burner process managed by an integration test.
type GPUBurner struct {
	statusURL string
	cmd       *exec.Cmd
	stderr    bytes.Buffer
}

func requireGPUBurner(t *testing.T) string {
	t.Helper()

	bin := os.Getenv(gpuBurnerBinEnv)
	if bin != "" {
		return bin
	}
	if os.Getenv("CI") != "" {
		t.Fatalf("%s must be set for GPU integration tests in CI", gpuBurnerBinEnv)
	}
	t.Skipf("%s is not set; skipping gpu-burner integration test", gpuBurnerBinEnv)
	return ""
}

// StartGPUBurner starts gpu-burner with the requested CUDA_VISIBLE_DEVICES value and
// waits until its status API reports running workers.
func StartGPUBurner(t *testing.T, visibleDevices string, workers int, targetSM int) *GPUBurner {
	t.Helper()
	require.NotEmpty(t, visibleDevices)

	command := strings.Fields(requireGPUBurner(t))
	require.NotEmpty(t, command)
	port := reserveLoopbackPort(t)
	ctx, cancel := context.WithCancel(t.Context())
	burner := &GPUBurner{
		statusURL: "http://127.0.0.1:" + strconv.Itoa(port) + "/status",
	}
	args := append(command[1:],
		"--api-host", "127.0.0.1",
		"--api-port", strconv.Itoa(port),
		"--run_time", strconv.Itoa(gpuBurnerRunTime),
		"--num_gpu_workers", strconv.Itoa(workers),
		"auto",
		"--target_sm", strconv.Itoa(targetSM),
		"--calibration_seconds", strconv.Itoa(gpuBurnerCalibrationSeconds),
		"--calibration_metric_sampling_interval", "0.2",
		"--calibration_max_iters", "3",
	)
	burner.cmd = exec.CommandContext(ctx, command[0], args...)
	burner.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	burner.cmd.Env = append(os.Environ(), "CUDA_VISIBLE_DEVICES="+visibleDevices)
	burner.cmd.Stderr = &burner.stderr
	t.Logf("starting gpu-burner: CUDA_VISIBLE_DEVICES=%q command=%s", visibleDevices, burner.cmd.String())
	require.NoError(t, burner.cmd.Start(), "start gpu-burner")

	t.Cleanup(func() {
		cancel()
		if burner.cmd.Process != nil {
			if err := syscall.Kill(-burner.cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
				t.Errorf("stop gpu-burner process group: %v", err)
			}
		}
		err := burner.cmd.Wait()
		if err != nil && ctx.Err() == nil {
			t.Errorf("gpu-burner exited unexpectedly: %v\nstderr:\n%s", err, burner.stderr.String())
		}
	})

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		status, err := burner.Status(t.Context())
		require.NoError(collect, err)
		require.Equal(collect, "running", status.Stage)
		require.Len(collect, status.Workers, workers)
		for _, worker := range status.Workers {
			require.Equal(collect, "running", worker.Stage)
			require.NotEmpty(collect, worker.GPUUUID)
			require.NotNil(collect, worker.Metrics)
		}
	}, gpuBurnerStartupLimit, time.Second, "gpu-burner did not become ready")

	return burner
}

// Status retrieves the current gpu-burner status API response.
func (burner *GPUBurner) Status(ctx context.Context) (GPUBurnerStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, burner.statusURL, nil)
	if err != nil {
		return GPUBurnerStatus{}, fmt.Errorf("create status request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return GPUBurnerStatus{}, fmt.Errorf("fetch gpu-burner status: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return GPUBurnerStatus{}, fmt.Errorf("gpu-burner status returned %s", response.Status)
	}

	var status GPUBurnerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return GPUBurnerStatus{}, fmt.Errorf("decode gpu-burner status: %w", err)
	}
	return status, nil
}

func reserveLoopbackPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "reserve loopback port")
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close(), "release loopback port")
	return port
}
