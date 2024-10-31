// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"os/exec"
	"path/filepath"
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

// RunSample executes the sample binary and returns the command. Cleanup is configured automatically
func RunSample(t *testing.T, name SampleName) (*exec.Cmd, error) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "..", "testdata", string(name)+".c")
	binaryFile := filepath.Join(curDir, "..", "testdata", string(name))

	builtBin, err := buildCBinary(sourceFile, binaryFile)
	require.NoError(t, err)

	cmd := exec.Command(builtBin)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	err = cmd.Start()
	require.NoError(t, err)

	log.Infof("Running sample binary %s with PID %d", name, cmd.Process.Pid)

	return cmd, nil
}
