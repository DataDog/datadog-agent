// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"fmt"
	"os/exec"
	"path/filepath"
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

func getBuiltSamplePath(t *testing.T, sample SampleName) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "..", "testdata", string(sample)+".c")
	binaryFile := filepath.Join(curDir, "..", "testdata", string(sample))

	builtBin, err := buildCBinary(sourceFile, binaryFile)
	require.NoError(t, err)

	return builtBin
}

// RunSample executes the sample binary and returns the command. Cleanup is configured automatically
func RunSample(t *testing.T, name SampleName) (*exec.Cmd, error) {
	builtBin := getBuiltSamplePath(t, name)

	log.Debugf("Running sample binary %s with command %v", name, builtBin)
	cmd := exec.Command(builtBin)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	err := cmd.Start()
	require.NoError(t, err)

	return cmd, nil
}

// RunSampleInDocker executes the sample binary in a Docker container and returns the PID of the main container process, and the container ID
func RunSampleInDocker(t *testing.T, name SampleName, image DockerImage) (int, string, error) {
	builtBin := getBuiltSamplePath(t, name)
	containerName := fmt.Sprintf("gpu-testutil-%s", utils.RandString(10))
	mountArg := fmt.Sprintf("%s:%s", builtBin, builtBin)

	command := []string{"docker", "run", "--rm", "-v", mountArg, "--name", containerName, string(image), builtBin}

	log.Debugf("Running sample binary %s with command %v", name, command)
	cmd := exec.Command(command[0], command[1:]...)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	err := cmd.Start()
	require.NoError(t, err)

	var dockerPID int64
	var dockerContainerID string
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

	return int(dockerPID), dockerContainerID, nil
}
