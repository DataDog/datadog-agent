// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func exitWith(code int) error {
	return exec.Command("cmd", "/c", fmt.Sprintf("exit %d", code)).Run()
}

func TestReloadRefusedExitCodeIsTheCLIContract(t *testing.T) {
	// RELOAD_REFUSED_EXIT_CODE in pkg/procmgr/rust/src/bins/dd-procmgr.rs has the same value.
	assert.Equal(t, 3, ddProcmgrReloadRefusedExitCode)
}

func TestReloadFailureActionFor(t *testing.T) {
	refused := exitWith(ddProcmgrReloadRefusedExitCode)

	// A refused reload never restarts the service.
	assert.Equal(t, stopProcess, reloadFailureActionFor(refused, DDOTProcmgrProcessName))
	assert.Equal(t, leaveRunning, reloadFailureActionFor(refused, ""))

	// Any other failure keeps the restart fallback.
	assert.Equal(t, restartService, reloadFailureActionFor(exitWith(1), DDOTProcmgrProcessName))
	assert.Equal(t, restartService, reloadFailureActionFor(errors.New("could not start"), DDOTProcmgrProcessName))
	assert.Equal(t, restartService, reloadFailureActionFor(exitWith(1), ""))
}

func TestDDOTProcmgrProcessNameIsTheConfigFileStem(t *testing.T) {
	assert.Equal(t, "datadog-agent-ddot", DDOTProcmgrProcessName)
	assert.Equal(t, DDOTProcmgrProcessName+".yaml", ddotProcmgrConfigFileName)
}

func TestIsReloadRefused(t *testing.T) {
	assert.False(t, isReloadRefused(nil))
	assert.False(t, isReloadRefused(errors.New("not an exit error")))
	assert.False(t, isReloadRefused(exitWith(1)))
	assert.True(t, isReloadRefused(exitWith(ddProcmgrReloadRefusedExitCode)))
	assert.True(t, isReloadRefused(fmt.Errorf("wrapped: %w", exitWith(ddProcmgrReloadRefusedExitCode))))
}
