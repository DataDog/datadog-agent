// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	_ "embed"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSetup(t *testing.T) {
	assert.Nil(t, BuildHelperForTests(os.TempDir(), os.TempDir(), false))
}

func TestInvalidCommands(t *testing.T) {
	testSetup(t)
	// assert wrong commands
	for input, expected := range map[string]string{
		// fail assert_command characters assertion
		";": "error: decoding command\n",
		"&": "error: decoding command\n",
		`{"command":"start", "unit":"does-not-exist"}`:                       "error: invalid unit\n",
		`{"command":"start", "unit":"datadog-//"}`:                           "error: invalid unit\n",
		`{"command":"does-not-exist", "unit":"datadog-"}`:                    "error: invalid command\n",
		`{"command":"chown dd-agent", "path":"/"}`:                           "error: invalid path\n",
		`{"command":"chown dd-agent", "path":"/opt/datadog-packages/../.."}`: "error: invalid path\n",
	} {
		assert.Equal(t, expected, executeCommand(input).Error())
	}
}

func TestAssertWorkingCommands(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping test on non-darwin OS")
	}
	testSetup(t)

	// missing permissions on test setup, e2e tests verify the successful commands
	successErr := "error: failed to lookup dd-updater user: user: unknown user dd-updater\n"

	require.Equal(t, successErr, startUnit("datadog-agent").Error())
	assert.Equal(t, successErr, stopUnit("datadog-agent").Error())
	assert.Equal(t, successErr, enableUnit("datadog-agent").Error())
	assert.Equal(t, successErr, disableUnit("datadog-agent").Error())
	assert.Equal(t, successErr, loadUnit("datadog-agent").Error())
	assert.Equal(t, successErr, removeUnit("datadog-agent").Error())
	assert.Equal(t, successErr, createAgentSymlink().Error())
	assert.Equal(t, successErr, rmAgentSymlink().Error())
}
