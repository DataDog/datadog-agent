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

// Sample represents a sample binary that can be run and tested
type Sample struct {
	Name            string
	StartPattern    *regexp.Regexp
	FinishedPattern *regexp.Regexp
	DefaultArgs     SampleArgs
}

// CudaSample is a binary that calls all the CUDA functions we probe for
var CudaSample = Sample{
	Name:            "cudasample",
	StartPattern:    regexp.MustCompile("Starting CudaSample program"),
	FinishedPattern: regexp.MustCompile("CUDA calls made."),
	DefaultArgs:     defaultCudaSampleArgs(),
}

// RateSample is a binary that calls the CUDA rate sample, allowing to launch CUDA calls at a given rate
var RateSample = Sample{
	Name:            "cudarate",
	StartPattern:    regexp.MustCompile("Starting CudaRateSample program"),
	FinishedPattern: regexp.MustCompile("CUDA calls made."),
	DefaultArgs:     defaultRateSampleArgs(),
}

// dockerImage represents the Docker image to use for running the sample binary.
type dockerImage string

const (
	// MinimalDockerImage is the minimal docker image, just used for running a binary
	MinimalDockerImage dockerImage = dockerutils.MinimalDockerImage
)

// SampleArgs is an interface that represents the arguments for the sample binary
type SampleArgs interface {
	Env() []string
	CLIArgs() []string
}

// CudaSampleArgs holds arguments for the sample binary
type CudaSampleArgs struct {
	// StartWaitTimeSec represents the time in seconds to wait before the binary starting the CUDA calls
	StartWaitTimeSec int

	// CudaVisibleDevicesEnv represents the value of the CUDA_VISIBLE_DEVICES environment variable
	CudaVisibleDevicesEnv string

	// SelectedDevice represents the device that the CUDA sample will select
	SelectedDevice int
}

// Env returns the environment variables for the CUDA sample binary
func (a *CudaSampleArgs) Env() []string {
	if a.CudaVisibleDevicesEnv != "" {
		return []string{"CUDA_VISIBLE_DEVICES=" + a.CudaVisibleDevicesEnv}
	}
	return nil
}

// CLIArgs returns the command line arguments for the CUDA sample binary
func (a *CudaSampleArgs) CLIArgs() []string {
	return []string{
		strconv.Itoa(a.StartWaitTimeSec),
		strconv.Itoa(a.SelectedDevice),
	}
}

// defaultCudaSampleArgs returns the default arguments for the CUDA sample binary
func defaultCudaSampleArgs() *CudaSampleArgs {
	return &CudaSampleArgs{
		StartWaitTimeSec:      5,
		CudaVisibleDevicesEnv: "",
		SelectedDevice:        0,
	}
}

// RateSampleArgs is an interface that represents the arguments for the CUDA rate sample binary
type RateSampleArgs struct {
	// StartWaitTimeSec represents the time in seconds to wait before the binary starting the CUDA calls
	StartWaitTimeSec int
	// SelectedDevice represents the device that the CUDA rate sample will select
	SelectedDevice int
	// CallsPerSecond represents the rate of CUDA calls per second
	CallsPerSecond int
	// ExecutionTimeSec represents the time in seconds to run the rate sample before exiting
	ExecutionTimeSec int
}

// Env returns the environment variables for the CUDA rate sample binary
func (a *RateSampleArgs) Env() []string {
	return nil
}

// CLIArgs returns the command line arguments for the CUDA rate sample binary
func (a *RateSampleArgs) CLIArgs() []string {
	return []string{strconv.Itoa(a.StartWaitTimeSec), strconv.Itoa(a.SelectedDevice), strconv.Itoa(a.CallsPerSecond), strconv.Itoa(a.ExecutionTimeSec)}
}

// defaultRateSampleArgs returns the default arguments for the CUDA rate sample binary
func defaultRateSampleArgs() *RateSampleArgs {
	return &RateSampleArgs{
		StartWaitTimeSec: 5,
		SelectedDevice:   0,
		CallsPerSecond:   1000,
		ExecutionTimeSec: 5,
	}
}

// RunSampleWithArgs executes the sample binary and returns the command. Cleanup is configured automatically
func getBuiltSamplePath(t testing.TB, sample Sample) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "..", "testdata", sample.Name+".c")
	binaryFile := filepath.Join(curDir, "..", "testdata", sample.Name)

	builtBin, err := buildCBinary(sourceFile, binaryFile)
	require.NoError(t, err)

	return builtBin
}

func runCommandAndPipeOutput(t testing.TB, command []string, sample Sample, args SampleArgs) (cmd *exec.Cmd, err error) {
	command = append(command, args.CLIArgs()...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cmd = exec.CommandContext(ctx, command[0], command[1:]...)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	scanner, err := procutil.NewScanner(sample.StartPattern, sample.FinishedPattern)
	require.NoError(t, err, "failed to create pattern scanner")
	defer func() {
		//print the cudasample log in case there was an error
		if err != nil {
			scanner.PrintLogs(t)
		}
	}()
	env := args.Env()
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
			err = fmt.Errorf("%s execution attempt reached timeout %v ", sample.Name, dockerutils.DefaultTimeout)
			return nil, err
		}
	}
}

// RunSample executes the sample binary and returns the command. Cleanup is configured automatically
func RunSample(t testing.TB, sample Sample) (*exec.Cmd, error) {
	return RunSampleWithArgs(t, sample, sample.DefaultArgs)
}

// RunSampleWithArgs executes the sample binary with args and returns the command. Cleanup is configured automatically
func RunSampleWithArgs(t testing.TB, sample Sample, args SampleArgs) (*exec.Cmd, error) {
	builtBin := getBuiltSamplePath(t, sample)
	return runCommandAndPipeOutput(t, []string{builtBin}, sample, args)
}

// RunSampleInDocker executes the sample binary in a Docker container and returns the PID of the main container process, and the container ID
func RunSampleInDocker(t testing.TB, sample Sample, image dockerImage) (int, string) {
	return RunSampleInDockerWithArgs(t, sample, image, sample.DefaultArgs)
}

// RunSampleInDockerWithArgs executes the sample binary in a Docker container and returns the PID of the main container process, and the container ID
func RunSampleInDockerWithArgs(t testing.TB, sample Sample, image dockerImage, args SampleArgs) (int, string) {
	builtBin := getBuiltSamplePath(t, sample)
	containerName := "gpu-testutil-" + utils.RandString(10)
	scanner, err := procutil.NewScanner(sample.StartPattern, sample.FinishedPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerConfig := dockerutils.NewRunConfig(
		dockerutils.NewBaseConfig(
			containerName,
			scanner,
			dockerutils.WithEnv(args.Env()),
		),
		string(image),
		builtBin,
		dockerutils.WithBinaryArgs(args.CLIArgs()),
		dockerutils.WithMounts(map[string]string{builtBin: builtBin}))

	require.NoError(t, dockerutils.Run(t, dockerConfig))

	var dockerPID int64
	var dockerContainerID string

	dockerPID, err = dockerutils.GetMainPID(containerName)
	assert.NoError(t, err, "failed to get docker PID")
	dockerContainerID, err = dockerutils.GetContainerID(containerName)
	assert.NoError(t, err, "failed to get docker container ID")

	log.Debugf("Sample binary %s running in Docker container %s (CID=%s) with PID %d", sample.Name, containerName, dockerContainerID, dockerPID)

	return int(dockerPID), dockerContainerID
}
