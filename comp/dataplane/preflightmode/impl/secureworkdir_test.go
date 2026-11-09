// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecureWorkDir lives in its own file because preflightmode_test.go is Unix-only — its fake
// ADP harness depends on Unix signals — which left the Windows branch of secureWorkDir with
// no unit coverage at all. Windows is the platform where the file mode does nothing and the
// ACL is the only protection, so it is the one that most needs a test to compile and run.
//
// The ACL mechanics are asserted in pkg/util/filesystem.TestRemoveAccessToOtherUsers. What
// this pins is that preflight mode can resolve the permissions to apply on the host it is running
// on and that applying them to a fresh directory succeeds — the failure mode being an
// unresolvable Agent SID, which would otherwise turn every Windows pre-flight into a
// spawn_failed at runtime.
func TestSecureWorkDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), workDirName)
	require.NoError(t, os.MkdirAll(dir, 0700))

	require.NoError(t, secureWorkDir(dir))

	// A file created afterwards must still be writable by us: on Windows it inherits the ACEs
	// secureWorkDir installed, and this is what proves those ACEs did not lock the Agent out
	// of its own working directory.
	cfgPath := filepath.Join(dir, preflightConfigFileName)
	require.NoError(t, os.WriteFile(cfgPath, []byte("api_key: secret\n"), 0600))

	content, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "api_key: secret\n", string(content))

	if runtime.GOOS == "windows" {
		// The mode is not an access control mechanism here, so there is nothing portable left
		// to assert; the DACL is the protection and it is covered in pkg/util/filesystem.
		return
	}
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

// TestSecureWorkDirMissingDirectory checks the error path is reported rather than swallowed,
// since prepare treats a failure here as a reason to abandon the run.
func TestSecureWorkDirMissingDirectory(t *testing.T) {
	assert.Error(t, secureWorkDir(filepath.Join(t.TempDir(), "does-not-exist")))
}
