// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func waitAndExec(opts options, stderr io.Writer) error {
	lock, err := os.OpenFile(opts.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open component lock: %w", err)
	}
	defer lock.Close()

	prepared, err := os.OpenFile(opts.preparedPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open Prepared marker: %w", err)
	}
	defer prepared.Close()

	if err := writePreparedMarker(prepared, opts.podUID); err != nil {
		return err
	}
	if err := setCloseOnExec(prepared.Fd(), false); err != nil {
		return fmt.Errorf("preserve Prepared marker across exec: %w", err)
	}

	fmt.Fprintf(stderr, "agent-rollout-gate: %s is Prepared; waiting for component lock\n", opts.component)
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("acquire component lock: %w", err)
	}
	if err := setCloseOnExec(lock.Fd(), false); err != nil {
		return fmt.Errorf("preserve component lock across exec: %w", err)
	}
	if opts.waitFile != "" {
		fmt.Fprintf(stderr, "agent-rollout-gate: %s acquired component lock; waiting for %s\n", opts.component, opts.waitFile)
		if err := waitForNonEmptyFile(opts.waitFile); err != nil {
			return err
		}
	}

	path, err := exec.LookPath(opts.command[0])
	if err != nil {
		return fmt.Errorf("resolve command after acquiring component lock: %w", err)
	}
	fmt.Fprintf(stderr, "agent-rollout-gate: %s acquired component lock; starting %s\n", opts.component, opts.command[0])
	if err := syscall.Exec(path, opts.command, os.Environ()); err != nil {
		return fmt.Errorf("exec command after acquiring component lock: %w", err)
	}
	return nil
}

func waitForNonEmptyFile(path string) error {
	for {
		info, err := os.Stat(path)
		switch {
		case err == nil && info.Mode().IsRegular() && info.Size() > 0:
			return nil
		case err == nil:
		case errors.Is(err, os.ErrNotExist):
		default:
			return fmt.Errorf("inspect required file %s: %w", path, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func writePreparedMarker(file *os.File, podUID string) error {
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate Prepared marker: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek Prepared marker: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%s %d %d\n", podUID, os.Getpid(), file.Fd()); err != nil {
		return fmt.Errorf("write Prepared marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync Prepared marker: %w", err)
	}
	return nil
}

func setCloseOnExec(fd uintptr, enabled bool) error {
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, uintptr(syscall.F_GETFD), 0)
	if errno != 0 {
		return errno
	}
	if enabled {
		flags |= syscall.FD_CLOEXEC
	} else {
		flags &^= syscall.FD_CLOEXEC
	}
	_, _, errno = syscall.Syscall(syscall.SYS_FCNTL, fd, uintptr(syscall.F_SETFD), flags)
	if errno != 0 {
		return errno
	}
	return nil
}
