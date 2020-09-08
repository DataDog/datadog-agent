// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build secrets,!windows

package secrets

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func setCorrectRight(path string) {
	os.Chmod(path, 0700)
}

func TestWrongPath(t *testing.T) {
	require.NotNil(t, checkRights("does not exists", false))
}

func TestGroupOtherRights(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())

	allowGroupExec := false

	// file exists
	require.NotNil(t, checkRights("/does not exists", allowGroupExec))

	require.Nil(t, os.Chmod(tmpfile.Name(), 0700))
	require.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// we should at least be able to execute it
	require.Nil(t, os.Chmod(tmpfile.Name(), 0100))
	require.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// owner have write permission
	require.Nil(t, os.Chmod(tmpfile.Name(), 0600))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// group should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0710))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// other should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0701))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	allowGroupExec = true

	// group should have only read and exec rights
	require.Nil(t, os.Chmod(tmpfile.Name(), 0750))
	require.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// group should not have write right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0770))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// other should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0751))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))
}
