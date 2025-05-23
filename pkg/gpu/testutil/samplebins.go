// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	procutil "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

// sampleName represents the name of the sample binary.
type sampleName string

const (
	// CudaSample is the sample binary that uses CUDA.
	CudaSample sampleName = "cudasample"
)

// dockerImage represents the Docker image to use for running the sample binary.
type dockerImage string

var startedPattern = regexp.MustCompile("Starting CudaSample program")
var finishedPattern = regexp.MustCompile("CUDA calls made")

const (
	// MinimalDockerImage is the minimal docker image, just used for running a binary
	MinimalDockerImage dockerImage = dockerutils.MinimalDockerImage
)

// SampleArgs holds arguments for the sample binary
type SampleArgs struct {
	// StartWaitTimeSec represents the time in seconds to wait before the binary starting the CUDA calls
	StartWaitTimeSec int

	// CudaVisibleDevicesEnv represents the value of the CUDA_VISIBLE_DEVICES environment variable
	CudaVisibleDevicesEnv string

	// SelectedDevice represents the device that the CUDA sample will select
	SelectedDevice int
}

func (a *SampleArgs) getEnv() []string {
	if a.CudaVisibleDevicesEnv != "" {
		return []string{fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", a.CudaVisibleDevicesEnv)}
	}
	return nil
}

func (a *SampleArgs) getCLIArgs() []string {
	return []string{
		strconv.Itoa(a.StartWaitTimeSec),
		strconv.Itoa(a.SelectedDevice),
	}
}

// RunSampleWithArgs executes the sample binary and returns the command. Cleanup is configured automatically
func getBuiltSamplePath(t *testing.T, sample sampleName) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "..", "testdata", string(sample)+".c")
	binaryFile := filepath.Join(curDir, "..", "testdata", string(sample))

	builtBin, err := buildCBinary(sourceFile, binaryFile)
	require.NoError(t, err)

	return builtBin
}

// getDefaultArgs returns the default arguments for the sample binary
func getDefaultArgs() SampleArgs {
	return SampleArgs{
		StartWaitTimeSec:      5,
		CudaVisibleDevicesEnv: "",
		SelectedDevice:        0,
	}
}

func runCommandAndPipeOutput(t *testing.T, command []string, args SampleArgs) (cmd *exec.Cmd, err error) {
	command = append(command, args.getCLIArgs()...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cmd = exec.CommandContext(ctx, command[0], command[1:]...)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	scanner, err := procutil.NewScanner(startedPattern, finishedPattern)
	require.NoError(t, err, "failed to create pattern scanner")
	defer func() {
		//print the cudasample log in case there was an error
		if err != nil {
			scanner.PrintLogs(t)
		}
	}()
	env := args.getEnv()
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdout = scanner
	cmd.Stderr = scanner

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	for {
		select {
		case <-ctx.Done():
			if err = ctx.Err(); err != nil {
				return nil, fmt.Errorf("failed to start the process %s due to: %w", command[0], err)
			}
		case <-scanner.DoneChan:
			t.Logf("%s command succeeded", command)
			return cmd, nil
		case <-time.After(dockerutils.DefaultTimeout):
			//setting the error explicitly to trigger the defer function
			err = fmt.Errorf("%s execution attempt reached timeout %v ", CudaSample, dockerutils.DefaultTimeout)
			return nil, err
		}
	}
}

// RunSample executes the sample binary and returns the command. Cleanup is configured automatically
func RunSample(t *testing.T, name sampleName) (*exec.Cmd, error) {
	return RunSampleWithArgs(t, name, getDefaultArgs())
}

// RunSampleWithArgs executes the sample binary with args and returns the command. Cleanup is configured automatically
func RunSampleWithArgs(t *testing.T, name sampleName, args SampleArgs) (*exec.Cmd, error) {
	builtBin := getBuiltSamplePath(t, name)
	return runCommandAndPipeOutput(t, []string{builtBin}, args)
}

// RunSampleInDocker executes the sample binary in a Docker container and returns the PID of the main container process, and the container ID
func RunSampleInDocker(t *testing.T, name sampleName, image dockerImage) (int, string) {
	return RunSampleInDockerWithArgs(t, name, image, getDefaultArgs())
}

// RunSampleInDockerWithArgs executes the sample binary in a Docker container and returns the PID of the main container process, and the container ID
func RunSampleInDockerWithArgs(t *testing.T, name sampleName, image dockerImage, args SampleArgs) (int, string) {
	builtBin := getBuiltSamplePath(t, name)
	containerName := fmt.Sprintf("gpu-testutil-%s", utils.RandString(10))
	scanner, err := procutil.NewScanner(startedPattern, finishedPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerConfig := dockerutils.NewRunConfig(
		dockerutils.NewBaseConfig(
			containerName,
			scanner,
			dockerutils.WithEnv(args.getEnv()),
		),
		string(image),
		builtBin,
		dockerutils.WithBinaryArgs(args.getCLIArgs()),
		dockerutils.WithMounts(map[string]string{builtBin: builtBin}))

	require.NoError(t, dockerutils.Run(t, dockerConfig))

	var dockerPID int64
	var dockerContainerID string

	dockerPID, err = dockerutils.GetMainPID(containerName)
	assert.NoError(t, err, "failed to get docker PID")
	dockerContainerID, err = dockerutils.GetContainerID(containerName)
	assert.NoError(t, err, "failed to get docker container ID")

	log.Debugf("Sample binary %s running in Docker container %s (CID=%s) with PID %d", name, containerName, dockerContainerID, dockerPID)

	return int(dockerPID), dockerContainerID
}
