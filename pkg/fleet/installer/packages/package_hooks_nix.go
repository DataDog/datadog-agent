// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func newPackageHookCommand(ctx context.Context, path string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, path)
	// A hook commonly launches a shell and children. Killing only the shell can
	// leave those children running and retaining the installer's output pipes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return cmd, nil
}
