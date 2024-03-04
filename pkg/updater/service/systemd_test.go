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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSetup(t *testing.T) {
	tmpDir := os.TempDir()
	updaterHelper = filepath.Join(tmpDir, "/updater-helper")
	cmd := exec.Command("go", "build", "-o", updaterHelper, "./helper/main.go")
	assert.Nil(t, cmd.Run())
}

func TestInvalidCommands(t *testing.T) {
	testSetup(t)
	// assert wrong commands
	for input, expected := range map[string]string{
		// fail assert_command characters assertion
		";":                    "error: invalid command\n",
		"&":                    "error: invalid command\n",
		"start does-not-exist": "error: invalid unit\n",
		"start a v c":          "error: missing unit\n",
	} {
		assert.Equal(t, expected, executeCommand(input).Error())
	}
}

func TestAssertWorkingCommands(t *testing.T) {
	testSetup(t)
	// missing permissions on test setup, e2e tests verify the successful commands
	successErr := "error: failed to lookup dd-agent user: user: unknown user dd-agent\n"

	require.Equal(t, successErr, startUnit("datadog-agent").Error())
	assert.Equal(t, successErr, stopUnit("datadog-agent").Error())
	assert.Equal(t, successErr, enableUnit("datadog-agent").Error())
	assert.Equal(t, successErr, disableUnit("datadog-agent").Error())
	assert.Equal(t, successErr, loadUnit("datadog-agent").Error())
	assert.Equal(t, successErr, removeUnit("datadog-agent").Error())
}
