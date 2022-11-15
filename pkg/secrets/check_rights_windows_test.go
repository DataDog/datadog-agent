// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build secrets && windows
// +build secrets,windows

package secrets

import (
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setCorrectRight(path string) {
	exec.Command("powershell", "test/setAcl.ps1",
		"-file", path,
		"-removeAllUser", "1",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "1").Run()
}

func TestWrongPath(t *testing.T) {
	require.NotNil(t, checkRights("does not exists", false))
}

func TestCheckRights(t *testing.T) {
	_, err := ioutil.TempFile("", "agent-collector-test")
	require.Nil(t, err)

	// default options
	allowGroupExec := false

	// file does not exist
	require.NotNil(t, checkRights("/does not exists", allowGroupExec))

	// missing current user
	tmpfile, err := ioutil.TempFile("", "agent-collector-test")
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
	tmpfile, err = ioutil.TempFile("", "agent-collector-test")
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
	tmpfile, err = ioutil.TempFile("", "agent-collector-test")
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
	tmpfile, err = ioutil.TempFile("", "agent-collector-test")
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
	tmpfile, err = ioutil.TempFile("", "agent-collector-test")
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
