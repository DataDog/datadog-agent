// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package scriptprocess

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

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

func TestRun_DeadlineAfterExitIsReported(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// The shell exits at once; the background sleep keeps stdout open until the deadline.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 &")

	start := time.Now()
	result, err := Run(ctx, cmd, 1024)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 0, result.ExitCode)
	assert.Less(t, time.Since(start), processWaitDelay, "run outlived its deadline")
}
