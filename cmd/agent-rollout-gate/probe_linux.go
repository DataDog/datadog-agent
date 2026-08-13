// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var signalActiveProcess = func(pid int, signal syscall.Signal) error {
	return syscall.Kill(pid, signal)
}

func runProbe(opts probeOptions) error {
	pid, live := liveMarkerPID(opts.activePath, opts.podUID)
	if !live {
		return errors.New("Active lease is not live")
	}
	healthErr := runHealthHandler(opts)
	if healthErr == nil {
		pending, err := enforcePendingRestart(opts, pid)
		if err != nil {
			return err
		}
		if pending {
			return errors.New("Active restart is already pending after exhausting the startup failure budget")
		}
		if err := resetStartupFailures(opts.activePath, opts.podUID); err != nil {
			return fmt.Errorf("reset Active startup failures: %w", err)
		}
		return nil
	}
	if err := recordStartupFailure(opts, pid); err != nil {
		return errors.Join(healthErr, err)
	}
	return healthErr
}

func markerOwnedByPod(path, podUID string) bool {
	_, live := liveMarkerPID(path, podUID)
	return live
}

func liveMarkerPID(path, podUID string) (int, bool) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var markerUID string
	var pid, fd int
	if n, err := fmt.Sscanf(string(contents), "%s %d %d", &markerUID, &pid, &fd); err != nil || n != 3 || markerUID != podUID || pid <= 0 || fd < 0 {
		return 0, false
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	descriptorInfo, err := os.Stat(fmt.Sprintf("/proc/%d/fd/%d", pid, fd))
	return pid, err == nil && os.SameFile(pathInfo, descriptorInfo)
}

func startupFailurePath(activePath, podUID string) string {
	return preparedMarkerPath(activePath+".startup-failures", podUID)
}

func resetStartupFailures(activePath, podUID string) error {
	err := os.Remove(startupFailurePath(activePath, podUID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func recordStartupFailure(opts probeOptions, pid int) error {
	path := startupFailurePath(opts.activePath, opts.podUID)
	state, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open Active startup failure state: %w", err)
	}
	defer state.Close()
	if err := syscall.Flock(int(state.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock Active startup failure state: %w", err)
	}

	var failures int
	var terminatedAtUnixNano int64
	contents, readErr := io.ReadAll(state)
	if readErr != nil {
		return fmt.Errorf("read Active startup failure state: %w", readErr)
	}
	if len(contents) != 0 {
		if n, scanErr := fmt.Sscanf(string(contents), "%d %d", &failures, &terminatedAtUnixNano); scanErr != nil || n != 2 || failures < 0 || terminatedAtUnixNano < 0 {
			failures = 0
			terminatedAtUnixNano = 0
		}
	}
	failures++
	now := time.Now()
	var signal syscall.Signal
	switch {
	case failures == opts.failureThreshold:
		signal = syscall.SIGTERM
		terminatedAtUnixNano = now.UnixNano()
	case failures > opts.failureThreshold && terminatedAtUnixNano > 0 && now.Sub(time.Unix(0, terminatedAtUnixNano)) >= opts.terminationGracePeriod:
		signal = syscall.SIGKILL
	}
	if err := state.Truncate(0); err != nil {
		return fmt.Errorf("truncate Active startup failure state: %w", err)
	}
	if _, err := state.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek Active startup failure state: %w", err)
	}
	if _, err := fmt.Fprintf(state, "%d %d\n", failures, terminatedAtUnixNano); err != nil {
		return fmt.Errorf("write Active startup failure state: %w", err)
	}
	if signal != 0 {
		if err := signalCurrentActiveProcess(opts, pid, signal); err != nil {
			return fmt.Errorf("signal Active process %d with %s after startup failure budget: %w", pid, signal, err)
		}
	}
	return nil
}

func enforcePendingRestart(opts probeOptions, pid int) (bool, error) {
	state, err := os.Open(startupFailurePath(opts.activePath, opts.podUID))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open Active startup failure state: %w", err)
	}
	defer state.Close()

	var failures int
	var terminatedAtUnixNano int64
	if n, scanErr := fmt.Fscanf(state, "%d %d", &failures, &terminatedAtUnixNano); scanErr != nil || n != 2 || terminatedAtUnixNano <= 0 {
		return false, nil
	}
	if time.Since(time.Unix(0, terminatedAtUnixNano)) >= opts.terminationGracePeriod {
		if err := signalCurrentActiveProcess(opts, pid, syscall.SIGKILL); err != nil {
			return true, fmt.Errorf("kill Active process %d after startup termination grace: %w", pid, err)
		}
	}
	return true, nil
}

func signalCurrentActiveProcess(opts probeOptions, pid int, signal syscall.Signal) error {
	activePID, live := liveMarkerPID(opts.activePath, opts.podUID)
	if !live || activePID != pid {
		return errors.New("Active lease changed before applying the startup failure budget")
	}
	return signalActiveProcess(pid, signal)
}

func runHealthHandler(opts probeOptions) error {
	switch opts.handler {
	case "active":
		return nil
	case "tcp":
		address := net.JoinHostPort(opts.address, strconv.Itoa(opts.port))
		connection, err := net.DialTimeout("tcp", address, opts.timeout)
		if err != nil {
			return fmt.Errorf("TCP health probe failed: %w", err)
		}
		return connection.Close()
	case "http":
		address := net.JoinHostPort(opts.address, strconv.Itoa(opts.port))
		connection, err := net.DialTimeout("tcp", address, opts.timeout)
		if err != nil {
			return fmt.Errorf("HTTP health probe failed: %w", err)
		}
		defer connection.Close()
		if err := connection.SetDeadline(time.Now().Add(opts.timeout)); err != nil {
			return fmt.Errorf("HTTP health probe failed: %w", err)
		}
		if _, err := fmt.Fprintf(connection, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", opts.path, address); err != nil {
			return fmt.Errorf("HTTP health probe failed: %w", err)
		}
		statusLine, err := bufio.NewReader(connection).ReadString('\n')
		if err != nil {
			return fmt.Errorf("HTTP health probe failed: %w", err)
		}
		var protocol string
		var statusCode int
		if n, err := fmt.Sscanf(strings.TrimSpace(statusLine), "%s %d", &protocol, &statusCode); err != nil || n != 2 || !strings.HasPrefix(protocol, "HTTP/") {
			return fmt.Errorf("HTTP health probe returned invalid status line %q", strings.TrimSpace(statusLine))
		}
		if statusCode < 200 || statusCode >= 400 {
			return fmt.Errorf("HTTP health probe returned status %d", statusCode)
		}
		return nil
	default:
		return fmt.Errorf("unsupported health handler %q", opts.handler)
	}
}
