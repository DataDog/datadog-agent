// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const processWaitDelay = 5 * time.Second

type unixCommandController struct {
	cmd *exec.Cmd
}

func platformCommand(command []string) ([]string, error) {
	return command, nil
}

func executionDeniedError(err error) error {
	return fmt.Errorf("authored-script execution was denied by the host; check file permissions and application-allowlisting policies such as fapolicyd: %w", err)
}

func startCommand(cmd *exec.Cmd) (commandController, error) {
	configureCommand(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &unixCommandController{cmd: cmd}, nil
}

func configureCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pgid = 0
	cmd.WaitDelay = processWaitDelay
}

func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func (c *unixCommandController) cancel() error {
	if c.cmd.Process == nil {
		return nil
	}
	if err := killProcessGroup(c.cmd.Process.Pid); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if killErr := c.cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
	}
	return nil
}

func (c *unixCommandController) terminate() error {
	if c.cmd.Process == nil {
		return nil
	}
	if err := killProcessGroup(c.cmd.Process.Pid); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
