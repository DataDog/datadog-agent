// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package scriptprocess

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigureCommand_PreservesProcessAttributes(t *testing.T) {
	cmd := exec.Command("/bin/echo", "hello")
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: "/sandbox"}

	configureCommand(cmd)

	assert.Equal(t, "/sandbox", cmd.SysProcAttr.Chroot)
	assert.True(t, cmd.SysProcAttr.Setpgid)
	assert.Zero(t, cmd.SysProcAttr.Pgid)
}
