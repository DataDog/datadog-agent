// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	processWaitDelay     = 5 * time.Second
	windowsJobExitCode   = 1
	powerShellExecutable = "powershell.exe"
)

type windowsCommandController struct {
	job windows.Handle
	cmd *exec.Cmd

	mutex      sync.Mutex
	terminated bool
	closed     bool
}

func platformCommand(command []string) ([]string, error) {
	entrypoint := command[0]
	extension := strings.ToLower(filepath.Ext(entrypoint))
	switch extension {
	case ".ps1":
		systemRoot, err := requiredWindowsEnvironmentVariable("SYSTEMROOT")
		if err != nil {
			return nil, err
		}
		powershell := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", powerShellExecutable)
		arguments := []string{
			powershell,
			"-NoLogo",
			"-NoProfile",
			"-NonInteractive",
			"-ExecutionPolicy",
			"Bypass",
			"-File",
			entrypoint,
		}
		return append(arguments, command[1:]...), nil
	case ".exe", ".com":
		return command, nil
	default:
		return nil, fmt.Errorf("authored-script command %q has unsupported Windows extension %q; expected .ps1, .exe, or .com", entrypoint, extension)
	}
}

func executionDeniedError(err error) error {
	return fmt.Errorf("authored-script execution was denied by the host; check file permissions, AppLocker, and Windows Defender Application Control policies: %w", err)
}

func startCommand(cmd *exec.Cmd) (commandController, error) {
	configureCommand(cmd)

	job, err := createWindowsJob()
	if err != nil {
		return nil, fmt.Errorf("could not create authored-script Windows job: %w", err)
	}
	controller := &windowsCommandController{job: job, cmd: cmd}
	started := false
	defer func() {
		if !started {
			_ = controller.close()
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if err := assignProcessToWindowsJob(job, cmd.Process.Pid); err != nil {
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		return nil, errors.Join(
			fmt.Errorf("could not isolate authored-script process in a Windows job: %w", err),
			ignoreProcessDone(killErr),
			ignoreExitError(waitErr),
		)
	}

	started = true
	return controller, nil
}

func configureCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
	cmd.WaitDelay = processWaitDelay
}

func createWindowsJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func assignProcessToWindowsJob(job windows.Handle, pid int) error {
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(job, process)
}

func (c *windowsCommandController) cancel() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := c.terminateLocked(); err != nil {
		var killErr error
		if c.cmd.Process != nil {
			killErr = ignoreProcessDone(c.cmd.Process.Kill())
		}
		return errors.Join(err, killErr, c.closeLocked())
	}
	return nil
}

func (c *windowsCommandController) terminate() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	terminateErr := c.terminateLocked()
	closeErr := c.closeLocked()
	return errors.Join(terminateErr, closeErr)
}

func (c *windowsCommandController) terminateLocked() error {
	if c.terminated || c.closed {
		return nil
	}
	if err := windows.TerminateJobObject(c.job, windowsJobExitCode); err != nil {
		return err
	}
	c.terminated = true
	return nil
}

func (c *windowsCommandController) close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.closeLocked()
}

func (c *windowsCommandController) closeLocked() error {
	if c.closed {
		return nil
	}
	if err := windows.CloseHandle(c.job); err != nil {
		return err
	}
	c.closed = true
	return nil
}

func ignoreProcessDone(err error) error {
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func ignoreExitError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}
