// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package sysfsroot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func withClass(t *testing.T) string {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "class"), 0o755))
	return dir
}

func TestDeviceSysFSRoot(t *testing.T) {
	// A container root with only the cgroup mount, as the Helm chart sets up.
	cgroupOnly := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(cgroupOnly, "fs", "cgroup"), 0o755))

	t.Run("root has devices", func(t *testing.T) {
		root := withClass(t)
		assert.Equal(t, root, deviceSysFSRoot(root, withClass(t)))
	})
	t.Run("root without devices falls back", func(t *testing.T) {
		fallback := withClass(t)
		assert.Equal(t, fallback, deviceSysFSRoot(cgroupOnly, fallback))
	})
	t.Run("missing root falls back", func(t *testing.T) {
		fallback := withClass(t)
		assert.Equal(t, fallback, deviceSysFSRoot(filepath.Join(cgroupOnly, "missing"), fallback))
	})
	t.Run("fallback without devices keeps root", func(t *testing.T) {
		assert.Equal(t, cgroupOnly, deviceSysFSRoot(cgroupOnly, t.TempDir()))
	})
	t.Run("root is the fallback", func(t *testing.T) {
		assert.Equal(t, cgroupOnly, deviceSysFSRoot(cgroupOnly, cgroupOnly))
	})
}

func TestDeviceSysFSRootHonorsHostSys(t *testing.T) {
	hostSys := withClass(t)
	t.Setenv("HOST_SYS", hostSys)
	kernel.ResetProcSysRootCachesForTest()
	ResetForTest()
	t.Cleanup(func() {
		kernel.ResetProcSysRootCachesForTest()
		ResetForTest()
	})

	assert.Equal(t, hostSys, DeviceSysFSRoot())
}
