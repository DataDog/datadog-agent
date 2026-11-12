// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package coredump

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func recoverNilDereference() (recovered any) {
	defer func() { recovered = recover() }()
	var pointer *int
	_ = *pointer
	return nil
}

func TestCCoreDumpPreservesGoSignalHandling(t *testing.T) {
	if os.Getenv("C_CORE_DUMP_TEST_CHILD") == "1" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var initialLimit unix.Rlimit
		require.NoError(t, unix.Getrlimit(unix.RLIMIT_CORE, &initialLimit))
		if initialLimit.Max != unix.RLIM_INFINITY {
			t.Skip("test requires permission to restore an unlimited core dump limit")
		}
		require.NoError(t, unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: initialLimit.Max}))

		config := configmock.New(t)
		config.SetInTest("go_core_dump", false)
		config.SetInTest("c_core_dump", true)
		require.NoError(t, Setup(config))

		var configuredLimit unix.Rlimit
		require.NoError(t, unix.Getrlimit(unix.RLIMIT_CORE, &configuredLimit))
		require.Equal(t, uint64(unix.RLIM_INFINITY), configuredLimit.Cur)

		require.NotNil(t, recoverNilDereference())
		require.NotNil(t, recoverNilDereference())
		return
	}

	command := exec.Command(os.Args[0], "-test.run=TestCCoreDumpPreservesGoSignalHandling", "-test.v")
	command.Env = append(os.Environ(), "C_CORE_DUMP_TEST_CHILD=1")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}
