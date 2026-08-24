// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func waitAndExec(opts options, stderr io.Writer) error {
	path, err := exec.LookPath(opts.command[0])
	if err != nil {
		return fmt.Errorf("resolve command before advertising Prepared: %w", err)
	}

	lock, err := os.OpenFile(opts.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open component lock: %w", err)
	}
	defer lock.Close()

	terminationSignals := make(chan os.Signal, 1)
	signal.Notify(terminationSignals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(terminationSignals)

	preparedPath := preparedMarkerPath(opts.preparedPath, opts.podUID)
	prepared, err := os.OpenFile(preparedPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open Prepared marker: %w", err)
	}
	defer prepared.Close()

	if err := writeLiveMarker(prepared, opts.podUID); err != nil {
		return fmt.Errorf("write Prepared marker: %w", err)
	}
	if err := setCloseOnExec(prepared.Fd(), false); err != nil {
		return fmt.Errorf("preserve Prepared marker across exec: %w", err)
	}

	fmt.Fprintf(stderr, "agent-rollout-gate: %s is Prepared; waiting for component lock\n", opts.component)
	if err := waitForComponentLock(lock, terminationSignals); err != nil {
		return fmt.Errorf("acquire component lock: %w", err)
	}
	signal.Stop(terminationSignals)
	if err := setCloseOnExec(lock.Fd(), false); err != nil {
		return fmt.Errorf("preserve component lock across exec: %w", err)
	}
	if opts.waitFile != "" {
		fmt.Fprintf(stderr, "agent-rollout-gate: %s acquired component lock; waiting for %s\n", opts.component, opts.waitFile)
		if err := waitForNonEmptyFile(opts.waitFile); err != nil {
			return err
		}
	}
	if err := resetStartupFailures(opts.activePath, opts.podUID); err != nil {
		return fmt.Errorf("reset startup failure state: %w", err)
	}

	active, err := os.OpenFile(opts.activePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open Active marker: %w", err)
	}
	defer active.Close()
	if err := writeLiveMarker(active, opts.podUID); err != nil {
		return fmt.Errorf("write Active marker: %w", err)
	}
	if err := setCloseOnExec(active.Fd(), false); err != nil {
		return fmt.Errorf("preserve Active marker across exec: %w", err)
	}
	// Active supersedes Prepared. Retire the Prepared lease only after Active is
	// live so startup/liveness always observes at least one valid state. Keeping
	// Prepared open after activation would let a lost Active marker masquerade as
	// a container which was still safely waiting for handoff.
	if err := prepared.Close(); err != nil {
		return fmt.Errorf("retire Prepared marker descriptor: %w", err)
	}
	if err := os.Remove(preparedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("retire Prepared marker path: %w", err)
	}

	fmt.Fprintf(stderr, "agent-rollout-gate: %s acquired component lock; starting %s\n", opts.component, opts.command[0])
	if err := syscall.Exec(path, opts.command, agentEnvironment(os.Environ())); err != nil {
		return fmt.Errorf("exec command after acquiring component lock: %w", err)
	}
	return nil
}

func waitForComponentLock(lock *os.File, terminationSignals <-chan os.Signal) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case received := <-terminationSignals:
			return fmt.Errorf("received %s", received)
		default:
		}

		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			select {
			case received := <-terminationSignals:
				return fmt.Errorf("received %s", received)
			default:
				return nil
			}
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}

		select {
		case received := <-terminationSignals:
			return fmt.Errorf("received %s", received)
		case <-ticker.C:
		}
	}
}

func preparedMarkerPath(basePath, podUID string) string {
	sum := sha256.Sum256([]byte(podUID))
	return fmt.Sprintf("%s.%x", basePath, sum[:8])
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

func writeLiveMarker(file *os.File, podUID string) error {
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate marker: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek marker: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%s %d %d\n", podUID, os.Getpid(), file.Fd()); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync marker: %w", err)
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
