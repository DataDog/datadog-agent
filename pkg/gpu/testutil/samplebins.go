// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	prototestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SampleName represents the name of the sample binary.
type SampleName string

const (
	// CudaSample is the sample binary that uses CUDA.
	CudaSample SampleName = "cudasample"
)

// DockerImage represents the Docker image to use for running the sample binary.
type DockerImage string

const (
	// MinimalDockerImage is the minimal docker image, just used for running a binary
	MinimalDockerImage DockerImage = "alpine:3.20.3"
)

type SampleArgs struct { //nolint:revive // TODO
	// StartWaitTimeSec represents the time in seconds to wait before the binary starting the CUDA calls
	StartWaitTimeSec int

	// EndWaitTimeSec represents the time in seconds to wait before the binary stops after making the CUDA calls
	// This is necessary because the mock CUDA calls are instant, which means that the binary will exit before the
	// eBPF probe has a chance to read the events and inspect the binary. To make the behavior of the sample binary
	// more predictable and avoid flakiness in the tests, we introduce a delay before the binary exits.
	EndWaitTimeSec int

	// CudaVisibleDevicesEnv represents the value of the CUDA_VISIBLE_DEVICES environment variable
	CudaVisibleDevicesEnv string

	// SelectedDevice represents the device that the CUDA sample will select
	SelectedDevice int
}

func (a *SampleArgs) getEnv() []string {
	env := []string{}
	if a.CudaVisibleDevicesEnv != "" {
		env = append(env, fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", a.CudaVisibleDevicesEnv))
	}
	return env
}

func (a *SampleArgs) getCLIArgs() []string {
	return []string{
		strconv.Itoa(int(a.StartWaitTimeSec)),
		strconv.Itoa(int(a.EndWaitTimeSec)),
		strconv.Itoa(a.SelectedDevice),
	}
}

// redirectReaderToLog reads from the reader and logs the output with the given prefix
func redirectReaderToLog(r io.Reader, prefix string) {
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			log.Debugf("%s: %s", prefix, scanner.Text())
		}
		// Automatically exits when the scanner reaches EOF, that is, when the command finishes
	}()
}

// RunSampleWithArgs executes the sample binary and returns the command. Cleanup is configured automatically
func getBuiltSamplePath(t *testing.T, sample SampleName) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "..", "testdata", string(sample)+".c")
	binaryFile := filepath.Join(curDir, "..", "testdata", string(sample))

	builtBin, err := buildCBinary(sourceFile, binaryFile)
	require.NoError(t, err)

	return builtBin
}

// GetDefaultArgs returns the default arguments for the sample binary
func GetDefaultArgs() SampleArgs {
	return SampleArgs{
		StartWaitTimeSec:      5,
		EndWaitTimeSec:        1, // We need the process to stay active a bit so we can inspect its environment variables, if it ends too quickly we get no information
		CudaVisibleDevicesEnv: "",
		SelectedDevice:        0,
	}
}

func runCommandAndPipeOutput(t *testing.T, command []string, args SampleArgs, logName string) *exec.Cmd {
	command = append(command, args.getCLIArgs()...)

	cmd := exec.Command(command[0], command[1:]...)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	env := args.getEnv()
	cmd.Env = append(cmd.Env, env...)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	redirectReaderToLog(stdout, fmt.Sprintf("%s stdout", logName))
	redirectReaderToLog(stderr, fmt.Sprintf("%s stderr", logName))

	log.Debugf("Running command %v, env=%v", command, env)
	err = cmd.Start()
	require.NoError(t, err)

	return cmd
}

// RunSample executes the sample binary and returns the command. Cleanup is configured automatically
func RunSample(t *testing.T, name SampleName) *exec.Cmd {
	return RunSampleWithArgs(t, name, GetDefaultArgs())
}

// RunSampleWithArgs executes the sample binary with args and returns the command. Cleanup is configured automatically
func RunSampleWithArgs(t *testing.T, name SampleName, args SampleArgs) *exec.Cmd {
	builtBin := getBuiltSamplePath(t, name)

	return runCommandAndPipeOutput(t, []string{builtBin}, args, string(name))
}

// RunSampleInDocker executes the sample binary in a Docker container and returns the PID of the main container process, and the container ID
func RunSampleInDocker(t *testing.T, name SampleName, image DockerImage) (int, string) {
	return RunSampleInDockerWithArgs(t, name, image, GetDefaultArgs())
}

// RunSampleInDockerWithArgs executes the sample binary in a Docker container and returns the PID of the main container process, and the container ID
func RunSampleInDockerWithArgs(t *testing.T, name SampleName, image DockerImage, args SampleArgs) (int, string) {
	builtBin := getBuiltSamplePath(t, name)
	containerName := fmt.Sprintf("gpu-testutil-%s", utils.RandString(10))
	mountArg := fmt.Sprintf("%s:%s", builtBin, builtBin)

	command := []string{"docker", "run", "--rm", "-v", mountArg, "--name", containerName}

	// Pass environment variables to the container as docker args
	for _, env := range args.getEnv() {
		command = append(command, "-e", env)
	}

	command = append(command, string(image), builtBin)

	_ = runCommandAndPipeOutput(t, command, args, string(name))

	var dockerPID int64
	var dockerContainerID string
	var err error
	// The docker container might take a bit to start, so we retry until we get the PID
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		dockerPID, err = prototestutil.GetDockerPID(containerName)
		assert.NoError(c, err)
	}, 1*time.Second, 100*time.Millisecond, "failed to get docker PID")

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		dockerContainerID, err = GetDockerContainerID(containerName)
		assert.NoError(c, err)
	}, 1*time.Second, 100*time.Millisecond, "failed to get docker container ID")

	log.Debugf("Sample binary %s running in Docker container %s (CID=%s) with PID %d", name, containerName, dockerContainerID, dockerPID)

	return int(dockerPID), dockerContainerID
}
