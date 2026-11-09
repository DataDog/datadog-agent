// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package scriptprocess

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

func TestConfigureCommandWindowsPreservesProcessAttributes(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	configureCommand(cmd)

	assert.True(t, cmd.SysProcAttr.HideWindow)
	assert.NotZero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP)
	assert.NotZero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED)
}
