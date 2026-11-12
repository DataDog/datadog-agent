// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package scriptprocess

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsJobExitCode = 1

type windowsCommandController struct {
	job windows.Handle
	cmd *exec.Cmd

	mu         sync.Mutex
	terminated bool
	closed     bool
}

func startCommand(cmd *exec.Cmd) (commandController, error) {
	job, err := createWindowsJob()
	if err != nil {
		return nil, fmt.Errorf("could not create script Windows job: %w", err)
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
			fmt.Errorf("could not isolate script process in a Windows job: %w", err),
			ignoreProcessDone(killErr),
			ignoreExitError(waitErr),
		)
	}
	if err := resumeProcessPrimaryThread(cmd.Process.Pid); err != nil {
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		return nil, errors.Join(
			fmt.Errorf("could not resume script process: %w", err),
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
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED
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

func resumeProcessPrimaryThread(pid int) error {
	// os/exec does not expose the primary thread handle returned by CreateProcess.
	// The process is still suspended here, so its only thread is the primary thread.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("could not snapshot Windows threads: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}

		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("could not open primary thread %d: %w", entry.ThreadID, err)
		}
		_, resumeErr := windows.ResumeThread(thread)
		closeErr := windows.CloseHandle(thread)
		return errors.Join(resumeErr, closeErr)
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("could not find initial thread for process %d", pid)
	}
	return err
}

func (c *windowsCommandController) cancel() error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
	c.mu.Lock()
	defer c.mu.Unlock()

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
	c.mu.Lock()
	defer c.mu.Unlock()
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
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return nil
	}
	return err
}
