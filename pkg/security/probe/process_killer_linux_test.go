// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"os"
	"testing"

	psutil "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

func TestProcessKillerLinuxKillReturnsProcfsErrorWhenCreateTimeFails(t *testing.T) {
	t.Setenv("HOST_PROC", t.TempDir())

	pid := os.Getpid()
	proc, err := psutil.NewProcess(int32(pid))
	require.NoError(t, err)

	createdAt, err := proc.CreateTime()
	require.Error(t, err)
	require.Zero(t, createdAt)

	err = (&ProcessKillerLinux{}).Kill(0, &killContext{
		pid:       pid,
		createdAt: 0,
	})

	require.EqualError(t, err, "process not found in procfs")
}
