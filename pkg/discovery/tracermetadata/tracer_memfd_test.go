// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package tracermetadata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestGetTracerMetadata(t *testing.T) {
	pid := os.Getpid()
	procfs := kernel.ProcFSRoot()

	t.Run("cpp", func(t *testing.T) {
		curDir, err := testutil.CurDir()
		require.NoError(t, err)
		testDataPath := filepath.Join(curDir, "testdata/tracer_cpp.data")
		data, err := os.ReadFile(testDataPath)
		require.NoError(t, err)
		createTracerMemfd(t, data)
		trm, err := GetTracerMetadata(pid, procfs)
		require.NoError(t, err)
		require.Equal(t, "my-service", trm.ServiceName)
		require.Equal(t, "f685d66a-7c12-4c47-84d0-c8ba75856374", trm.RuntimeID)
		require.Equal(t, "cpp", trm.TracerLanguage)
		require.Equal(t, "v1.0.0", trm.TracerVersion)
		require.Equal(t, "my-hostname", trm.Hostname)
		require.Equal(t, "my-env", trm.ServiceEnv)
		require.Equal(t, "my-version", trm.ServiceVersion)
		require.Equal(t, uint8(1), trm.SchemaVersion)
	})

	t.Run("invalid data", func(t *testing.T) {
		createTracerMemfd(t, []byte("invalid data"))
		_, err := GetTracerMetadata(pid, procfs)
		require.Error(t, err)
	})
}

func createTracerMemfd(t *testing.T, l []byte) {
	t.Helper()
	fd, err := unix.MemfdCreate("datadog-tracer-info-xxx", 0)
	require.NoError(t, err)
	t.Cleanup(func() { unix.Close(fd) })
	err = unix.Ftruncate(fd, int64(len(l)))
	require.NoError(t, err)
	data, err := unix.Mmap(fd, 0, len(l), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	require.NoError(t, err)
	copy(data, l)
	err = unix.Munmap(data)
	require.NoError(t, err)
}
