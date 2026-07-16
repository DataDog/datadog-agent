// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package slurm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// writeFakeEnviron writes a /proc/<pid>/environ file (null-separated) under procRoot.
func writeFakeEnviron(tb testing.TB, procRoot string, pid int, vars []string) {
	pidDir := filepath.Join(procRoot, strconv.Itoa(pid))
	require.NoError(tb, os.MkdirAll(pidDir, 0755))
	content := strings.Join(vars, "\x00") + "\x00"
	require.NoError(tb, os.WriteFile(filepath.Join(pidDir, "environ"), []byte(content), 0644))
}

func TestGetSlurmInfo_EnvironPrimarySignal(t *testing.T) {
	procRoot := t.TempDir()
	kernel.WithFakeProcFS(t, procRoot)

	writeFakeEnviron(t, procRoot, 4242, []string{
		"PATH=/usr/bin",
		"SLURM_JOB_ID=3",
		"SLURM_JOB_NAME=gpuhold",
		"SLURM_JOB_PARTITION=gpu",
		"SLURM_VERSION=26.05.1",
	})

	p := &procProvider{}
	info, err := p.GetSlurmInfo(4242)
	require.NoError(t, err)
	assert.Equal(t, "3", info.JobID)
	assert.Equal(t, "gpuhold", info.JobName)
	assert.Equal(t, "gpu", info.Partition)
}

func TestGetSlurmInfo_NotASlurmJob(t *testing.T) {
	procRoot := t.TempDir()
	kernel.WithFakeProcFS(t, procRoot)

	writeFakeEnviron(t, procRoot, 6464, []string{"PATH=/usr/bin"})

	p := &procProvider{}
	info, err := p.GetSlurmInfo(6464)
	require.NoError(t, err) // absence is not an error
	assert.Empty(t, info.JobID)
}

func TestGetSlurmInfo_PermissionDeniedIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based permission denial doesn't apply when running as root")
	}

	procRoot := t.TempDir()
	kernel.WithFakeProcFS(t, procRoot)

	pidDir := filepath.Join(procRoot, "7575")
	require.NoError(t, os.MkdirAll(pidDir, 0755))
	// environ exists but is unreadable, so the only signal (SYS_PTRACE-gated) fails permission.
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "environ"), []byte("x"), 0000))

	p := &procProvider{}
	_, err := p.GetSlurmInfo(7575)
	require.Error(t, err)
}
