// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	_ "embed"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setFifoPaths(t *testing.T) string {
	tmpDir := t.TempDir()
	inFifoPath = tmpDir + "/run/in.fifo"
	outFifoPath = tmpDir + "/run/out.fifo"
	assert.Nil(t, exec.Command("mkdir", "-p", tmpDir+"/run").Run())
	return tmpDir
}

func runAdmin(t *testing.T, updaterTestPath string) <-chan int {
	template, err := os.ReadFile("../../../omnibus/config/templates/updater/updater-admin-exec.sh.erb")
	assert.Nil(t, err)
	runScript := strings.Replace(string(template), "<%= install_dir %>", updaterTestPath, -1)

	done := make(chan int)
	go func() {
		cmd := exec.Command("/bin/sh", "-c", string(runScript))
		cmd.Dir = updaterTestPath
		assert.Nil(t, cmd.Run())
		done <- 1
	}()
	return done
}

func TestScriptRunnerBootAndCleanup(t *testing.T) {
	setFifoPaths(t)

	// installing fake fifo files to assert cleanup at newScriptRunner
	f, err := os.Create(inFifoPath)
	assert.Nil(t, err)
	assert.Nil(t, f.Close())
	f, err = os.Create(outFifoPath)
	assert.Nil(t, err)
	assert.Nil(t, f.Close())

	s, err := newScriptRunner()
	assert.Nil(t, err)
	require.NotNil(t, s)

	fileInfo, err := os.Stat(inFifoPath)
	assert.Equal(t, fileInfo.Mode()&os.ModeNamedPipe, os.ModeNamedPipe)
	assert.Nil(t, err)

	fileInfo, err = os.Stat(outFifoPath)
	assert.Equal(t, fileInfo.Mode()&os.ModeNamedPipe, os.ModeNamedPipe)
	assert.Nil(t, err)

	s.close()
	_, err = os.Stat(inFifoPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(outFifoPath)
	assert.True(t, os.IsNotExist(err))
}

func TestInvalidCommands(t *testing.T) {
	updaterPath := setFifoPaths(t)
	s, err := newScriptRunner()
	assert.Nil(t, err)
	require.NotNil(t, s)

	done := runAdmin(t, updaterPath)

	// assert wrong commands
	for input, expected := range map[string]string{
		// fail assert_command max size
		strings.Repeat("a", 101): "error executing command " + strings.Repeat("a", 101) + ": command longer than 100",
		// fail assert_command characters assertion
		";": "error executing command ;: invalid command: ;",
		"&": "error executing command &: invalid command: &",
		"/": "error executing command /: invalid command: /",

		// fail command does not exist
		"echo hello": "error executing command echo hello: not supported command: echo",
	} {
		assert.Equal(t, expected, s.executeCommand(input).Error())
	}

	s.close()
	select {
	case <-time.After(30 * time.Second):
		t.Fatal("timeout")
	case <-done:
	}
}
