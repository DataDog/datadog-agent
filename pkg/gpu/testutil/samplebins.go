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

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SampleName represents the name of the sample binary.
type SampleName string

const (
	// CudaSample is the sample binary that uses CUDA.
	CudaSample SampleName = "cudasample"
)

type SampleArgs struct {
	// StartWaitTimeSec represents the time in seconds to wait before the binary starting the CUDA calls
	StartWaitTimeSec int

	// EndWaitTimeSec represents the time in seconds to wait before the binary stops after making the CUDA calls
	// This is necessary because the mock CUDA calls are instant, which means that the binary will exit before the
	// eBPF probe has a chance to read the events and inspect the binary. To make the behavior of the sample binary
	// more predictable and avoid flakiness in the tests, we introduce a delay before the binary exits.
	EndWaitTimeSec int

	// CudaVisibleDevicesEnv represents the value of the CUDA_VISIBLE_DEVICES environment variable
	CudaVisibleDevicesEnv string

	// DeviceToSelect represents the device that the CUDA sample will select
	DeviceToSelect int
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
		strconv.Itoa(a.DeviceToSelect),
	}
}

func RunSample(t *testing.T, name SampleName) (*exec.Cmd, error) {
	args := SampleArgs{
		StartWaitTimeSec:      5,
		EndWaitTimeSec:        1, // We need the process to stay active a bit so we can inspect its environment variables, if it ends too quickly we get no information
		CudaVisibleDevicesEnv: "",
		DeviceToSelect:        0,
	}
	return RunSampleWithArgs(t, name, args)
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
func RunSampleWithArgs(t *testing.T, name SampleName, args SampleArgs) (*exec.Cmd, error) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "..", "testdata", string(name)+".c")
	binaryFile := filepath.Join(curDir, "..", "testdata", string(name))

	builtBin, err := buildCBinary(sourceFile, binaryFile)
	require.NoError(t, err)

	cliArgs := args.getCLIArgs()
	env := args.getEnv()
	cmd := exec.Command(builtBin, cliArgs...)
	cmd.Env = append(cmd.Env, env...)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	redirectReaderToLog(stdout, fmt.Sprintf("%s stdout", name))
	redirectReaderToLog(stderr, fmt.Sprintf("%s stderr", name))

	log.Debugf("Running sample binary %s with args=%v, env=%v", name, cliArgs, env)
	err = cmd.Start()
	require.NoError(t, err)

	log.Infof("Running sample binary %s with PID %d", name, cmd.Process.Pid)

	return cmd, nil
}
