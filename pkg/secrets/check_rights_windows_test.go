// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build secrets && windows

package secrets

import (
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func setCorrectRight(path string) {
	exec.Command("powershell", "test/setAcl.ps1",
		"-file", path,
		"-removeAllUser", "1",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "1").Run()
}

func testCheckRightsStub() {
	// Stub for CI since running as Administrator and no installer data
	getDDAgentUserSID = winutil.GetSidFromUser
}

func TestWrongPath(t *testing.T) {
	require.NotNil(t, checkRights("does not exists", false))
}

func TestSpaceInPath(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "super temp")
	require.Nil(t, err)
	defer os.Remove(tmpDir)
	tmpFile, err := os.CreateTemp(tmpDir, "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpFile.Name())
	require.Nil(t, os.Chmod(tmpFile.Name(), 0700))
	require.Nil(t, checkRights(tmpFile.Name(), false))
}

func TestCheckRights(t *testing.T) {
	// default options
	allowGroupExec := false

	// file does not exist
	require.NotNil(t, checkRights("/does not exists", allowGroupExec))

	// missing current user
	tmpfile, err := os.CreateTemp("", "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())

	exec.Command("powershell", "test/setAcl.ps1",
		"-file", tmpfile.Name(),
		"-removeAllUser", "1",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "0").Run()
	assert.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// missing localSystem
	tmpfile, err = os.CreateTemp("", "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())
	exec.Command("powershell", "test/setAcl.ps1",
		"-file", tmpfile.Name(),
		"-removeAllUser", "1",
		"-removeAdmin", "0",
		"-removeLocalSystem", "1",
		"-addDDuser", "0").Run()
	assert.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// missing Administrator
	tmpfile, err = os.CreateTemp("", "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())
	exec.Command("powershell", "test/setAcl.ps1",
		"-file", tmpfile.Name(),
		"-removeAllUser", "1",
		"-removeAdmin", "1",
		"-removeLocalSystem", "0",
		"-addDDuser", "0").Run()
	assert.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// extra rights for someone else
	tmpfile, err = os.CreateTemp("", "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())
	exec.Command("powershell", "test/setAcl.ps1",
		"-file", tmpfile.Name(),
		"-removeAllUser", "0",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "1").Run()
	assert.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// missing localSystem or Administrator
	tmpfile, err = os.CreateTemp("", "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())
	exec.Command("powershell", "test/setAcl.ps1",
		"-file", tmpfile.Name(),
		"-removeAllUser", "1",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "1").Run()
	assert.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))
}
