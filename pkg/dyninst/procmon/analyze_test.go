// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func makeEnviron(t *testing.T, keyVals ...string) []byte {
	require.Truef(
		t, len(keyVals)%2 == 0,
		"keyVals must be a multiple of 2, got %d: %v", len(keyVals), keyVals,
	)
	var buf bytes.Buffer
	for i := 0; i < len(keyVals); i += 2 {
		fmt.Fprintf(&buf, "%s=%s\x00", keyVals[i], keyVals[i+1])
	}
	return buf.Bytes()
}

func TestAnalyzeProcess(t *testing.T) {
	envTrue := makeEnviron(t,
		ddDynInstEnabledEnvVar, "true",
		ddServiceEnvVar, "foo",
	)
	envFalse := makeEnviron(t,
		"FOO", "bar",
	)

	const exeTargetName = "exe_target"

	// makeProcFS creates a minimal on-disk proc-like structure under a temp dir
	// and returns the path that should be used as procfsRoot when calling
	// buildUpdate.
	makeProcFS := func(
		t *testing.T, pid uint32, env []byte, withExe bool,
	) (
		tmpDir string, procRoot string, cleanup func(),
	) {
		tmpDir = t.TempDir()
		procRoot = filepath.Join(tmpDir, "proc")

		procDir := filepath.Join(procRoot, strconv.Itoa(int(pid)))
		require.NoError(t, os.MkdirAll(procDir, 0o755))

		// /proc/<pid>/environ
		require.NoError(t, os.WriteFile(filepath.Join(procDir, "environ"), env, 0o644))

		if withExe {
			// create a fake executable file and a symlink named "exe" pointing to it
			exeTarget := filepath.Join(tmpDir, exeTargetName)
			require.NoError(t, os.WriteFile(exeTarget, []byte{}, 0o755))
			require.NoError(t, os.Symlink(exeTarget, filepath.Join(procDir, "exe")))
		}

		return tmpDir, procRoot, func() {
			os.RemoveAll(tmpDir)
		}
	}

	t.Run("not interesting env", func(t *testing.T) {
		_, procRoot, cleanup := makeProcFS(t, 102, envFalse, true)
		defer cleanup()
		res, err := analyzeProcess(102, procRoot)
		require.NoError(t, err)
		require.False(t, res.interesting)
	})

	t.Run("no interesting exe", func(t *testing.T) {
		_, procRoot, cleanup := makeProcFS(t, 101, envTrue, true)
		defer cleanup()
		res, err := analyzeProcess(101, procRoot)
		require.NoError(t, err)
		require.False(t, res.interesting)
		require.Empty(t, res.exe)
	})

	t.Run("exe missing", func(t *testing.T) {
		_, procRoot, cleanup := makeProcFS(t, 103, envTrue, false)
		defer cleanup()
		res, err := analyzeProcess(103, procRoot)
		require.Regexp(t, "failed to open exe link for pid 103.*: no such file or directory", err)
		require.False(t, res.interesting)
	})

	t.Run("interesting", func(t *testing.T) {
		cfgs := testprogs.MustGetCommonConfigs(t)
		bin := testprogs.MustGetBinary(t, "simple", cfgs[0])

		tmpDir, procRoot, cleanup := makeProcFS(t, 104, envTrue, true)
		defer cleanup()
		exeTarget := filepath.Join(tmpDir, exeTargetName)
		require.NoError(t, os.Remove(exeTarget), "failed to remove exe target", exeTarget)
		{
			f, err := os.Create(exeTarget)
			require.NoError(t, err)
			binReader, err := os.Open(bin)
			require.NoError(t, err)
			_, err = io.Copy(f, binReader)
			require.NoError(t, err)
			require.NoError(t, f.Close())
			require.NoError(t, binReader.Close())
		}
		res, err := analyzeProcess(104, procRoot)
		require.NoError(t, err)
		require.True(t, res.interesting)
		require.NotEmpty(t, res.exe.Path)
		require.Equal(t, "foo", res.service)
	})
}
