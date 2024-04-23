// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"context"
	_ "embed"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCtx = context.TODO()

func testSetup(t *testing.T) {
	assert.Nil(t, BuildHelperForTests(os.TempDir(), os.TempDir(), false))
}

func TestInvalidCommands(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("FIXME: broken on darwin")
	}

	testSetup(t)
	// assert wrong commands
	for input, expected := range map[string]string{
		// fail assert_command characters assertion
		";": "error: decoding command ;\n",
		"&": "error: decoding command &\n",
		`{"command":"start", "unit":"does-not-exist"}`:                       "error: invalid unit\n",
		`{"command":"start", "unit":"datadog-//"}`:                           "error: invalid unit\n",
		`{"command":"does-not-exist", "unit":"datadog-"}`:                    "error: invalid command\n",
		`{"command":"chown dd-agent", "path":"/"}`:                           "error: invalid path\n",
		`{"command":"chown dd-agent", "path":"/opt/datadog-packages/../.."}`: "error: invalid path\n",
	} {
		assert.Equal(t, expected, executeHelperCommand(testCtx, input).Error())
	}
}

func TestAssertWorkingCommands(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping test on non-darwin OS")
	}
	t.Skip("FIXME")
	testSetup(t)

	// missing permissions on test setup, e2e tests verify the successful commands
	successErr := "error: failed to lookup dd-agent user: user: unknown user dd-agent\n"
	successSystemd := "error: systemd unit path error: stat /lib/systemd/system: no such file or directory\n"

	require.Equal(t, successErr, startUnit(testCtx, "datadog-agent").Error())
	assert.Equal(t, successErr, stopUnit(testCtx, "datadog-agent").Error())
	assert.Equal(t, successErr, enableUnit(testCtx, "datadog-agent").Error())
	assert.Equal(t, successErr, disableUnit(testCtx, "datadog-agent").Error())
	assert.Equal(t, successSystemd, loadUnit(testCtx, "datadog-agent").Error())
	assert.Equal(t, successSystemd, removeUnit(testCtx, "datadog-agent").Error())
	assert.Equal(t, successErr, createAgentSymlink(testCtx).Error())
	assert.Equal(t, successErr, rmAgentSymlink(testCtx).Error())
	assert.Equal(t, successErr, backupAgentConfig(testCtx).Error())
	assert.Equal(t, successErr, restoreAgentConfig(testCtx).Error())

	a := &apmInjectorInstaller{
		installPath: "/tmp/stable",
	}
	assert.Equal(t, successErr, replaceLDPreload(testCtx).Error())
	assert.Equal(t, successErr, a.setAgentConfig(testCtx).Error())
	assert.Equal(t, successErr, a.setDockerConfig(testCtx).Error())
}
