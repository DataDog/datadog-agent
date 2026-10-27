// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package privilegedhelper

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func stubStart(t *testing.T, landlock int) (execPath *string, execArgs *[]string) {
	execPath, execArgs = new(string), new([]string)
	originalLandlock, originalExec := landlockABIVersion, execve
	landlockABIVersion = func() (int, error) { return landlock, nil }
	execve = func(path string, argv []string, _ []string) error {
		*execPath, *execArgs = path, argv
		return errors.New("exec stubbed")
	}
	t.Cleanup(func() { landlockABIVersion, execve = originalLandlock, originalExec })
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
