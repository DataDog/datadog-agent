// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package privilegedhelper

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func stubStart(t *testing.T, landlock int) (execPath *string, execArgs *[]string) {
	execPath, execArgs = new(string), new([]string)
	originalLandlock, originalExec, originalTrust := landlockABIVersion, execve, ensureTrustedHelper
	landlockABIVersion = func() (int, error) { return landlock, nil }
	execve = func(path string, argv []string, _ []string) error {
		*execPath, *execArgs = path, argv
		return errors.New("exec stubbed")
	}
	ensureTrustedHelper = func(string) error { return nil }
	t.Cleanup(func() {
		landlockABIVersion, execve, ensureTrustedHelper = originalLandlock, originalExec, originalTrust
	})
	return execPath, execArgs
}

func TestStartHelperSkipsUnsupportedKernels(t *testing.T) {
	_, execArgs := stubStart(t, minLandlockABI-1)

	require.NoError(t, startHelper(logmock.New(t), &helperParams{helperPath: "/opt/rshell"}))

	assert.Nil(t, *execArgs, "the helper must not start")
}

func TestStartHelperExecsTheHelper(t *testing.T) {
	execPath, execArgs := stubStart(t, minLandlockABI)

	err := startHelper(logmock.New(t), &helperParams{helperPath: "/opt/rshell", socketPath: "/run/h.sock", policyPath: "/etc/policy.json"})
	require.ErrorContains(t, err, "exec stubbed")

	assert.Equal(t, "/opt/rshell", *execPath)
	assert.Equal(t, []string{"/opt/rshell", "privileged-helper", "--socket=/run/h.sock", "--policy=/etc/policy.json", "--idle-timeout=0"}, *execArgs)
}

func TestEnsureTrustedHelperBinaryRejectsUntrusted(t *testing.T) {
	dir := t.TempDir()

	// A symlink is rejected even if its target is a valid regular file: the
	// target could be swapped after the ownership check.
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/true\n"), 0755))
	link := filepath.Join(dir, "rshell-link")
	require.NoError(t, os.Symlink(target, link))
	assert.ErrorContains(t, ensureTrustedHelperBinary(link), "not a regular file")

	// A missing binary is rejected.
	assert.Error(t, ensureTrustedHelperBinary(filepath.Join(dir, "absent")))

	// A regular file not owned by root is rejected. When the test runs as a
	// non-root user, the file it creates is owned by that user.
	if os.Geteuid() != 0 {
		regular := filepath.Join(dir, "regular")
		require.NoError(t, os.WriteFile(regular, []byte("#!/bin/true\n"), 0755))
		assert.ErrorContains(t, ensureTrustedHelperBinary(regular), "not root")
	}
}

func TestEnsureTrustedHelperBinaryRejectsWritable(t *testing.T) {
	// The group/other-writable check only runs once ownership is root, so this
	// case is meaningful only when the test itself runs as root.
	if os.Geteuid() != 0 {
		t.Skip("needs root to create a root-owned file and reach the mode check")
	}
	dir := t.TempDir()
	writable := filepath.Join(dir, "writable")
	require.NoError(t, os.WriteFile(writable, []byte("#!/bin/true\n"), 0757))
	assert.ErrorContains(t, ensureTrustedHelperBinary(writable), "writable by group or other")

	good := filepath.Join(dir, "good")
	require.NoError(t, os.WriteFile(good, []byte("#!/bin/true\n"), 0755))
	assert.NoError(t, ensureTrustedHelperBinary(good))
}
