// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && test

package uprobes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFmapperRunnerStopReapsProcess(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())

	runner := &FmapperRunner{cmd: cmd}
	pid := runner.GetTargetPid(t)
	runner.Stop(t)

	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	require.ErrorIs(t, err, os.ErrNotExist, "fmapper process should be reaped")
}
