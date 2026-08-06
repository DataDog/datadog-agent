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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const gateHelperEnv = "DD_AGENT_ROLLOUT_GATE_TEST_HELPER"

func TestGateHelperProcess(t *testing.T) {
	if os.Getenv(gateHelperEnv) != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			if err := run(os.Args[i+1:], os.Stderr); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing gate argument separator")
}

func TestWritePreparedMarkerIdentifiesTheOpenDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "prepared")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := writePreparedMarker(file, "pod-uid"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("pod-uid %d %d", os.Getpid(), file.Fd())
	if strings.TrimSpace(string(contents)) != want {
		t.Fatalf("got %q, want %q", contents, want)
	}
}

func TestWaitForNonEmptyFile(t *testing.T) {
	path := t.TempDir() + "/token"
	done := make(chan error, 1)
	go func() { done <- waitForNonEmptyFile(path) }()

	select {
	case err := <-done:
		t.Fatalf("returned before the file existed: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := os.WriteFile(path, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("did not return after the file became non-empty")
	}
}

func TestMissingCommandIsNotCheckedUntilAfterLockAcquisition(t *testing.T) {
	dir := t.TempDir()
	opts := options{
		component:    "agent",
		lockPath:     dir + "/agent.lock",
		preparedPath: dir + "/agent.prepared",
		podUID:       "pod-uid",
		waitFile:     dir + "/missing-token",
		command:      []string{dir + "/missing-agent"},
	}

	oldLock, err := os.OpenFile(opts.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer oldLock.Close()
	if err := syscall.Flock(int(oldLock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- waitAndExec(opts, io.Discard) }()
	waitForFileContents(t, opts.preparedPath)
	select {
	case err := <-done:
		t.Fatalf("gate checked the command before acquiring the lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := syscall.Flock(int(oldLock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "resolve command after acquiring component lock") {
			t.Fatalf("got %v, want post-lock command error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gate did not check the command after lock acquisition")
	}
}

func TestSetCloseOnExec(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "fd")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := setCloseOnExec(file.Fd(), false); err != nil {
		t.Fatal(err)
	}
	if err := setCloseOnExec(file.Fd(), true); err != nil {
		t.Fatal(err)
	}
}

func TestLockAndMarkerDescriptorsSurviveExec(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "agent.lock")
	preparedPath := filepath.Join(dir, "agent.prepared")

	oldLock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer oldLock.Close()
	if err := syscall.Flock(int(oldLock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	stdin, stdinWriter := io.Pipe()
	defer stdinWriter.Close()
	command := exec.Command(os.Args[0],
		"-test.run=^TestGateHelperProcess$", "--",
		"--component", "agent", "--", "sh", "-c",
		`read uid pid fd < "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH" && test "$pid" = "$$" && test "/proc/$$/fd/$fd" -ef "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH" && echo exec-ready && read line`,
	)
	command.Env = append(os.Environ(),
		gateHelperEnv+"=1",
		lockPathEnv+"="+lockPath,
		preparedPathEnv+"="+preparedPath,
		podUIDEnv+"=replacement-pod",
	)
	command.Stdin = stdin
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()

	waitForFileContents(t, preparedPath)
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		ready <- strings.TrimSpace(line)
	}()
	select {
	case line := <-ready:
		t.Fatalf("gate executed while the old lock was held, output %q", line)
	case <-time.After(150 * time.Millisecond):
	}

	if err := syscall.Flock(int(oldLock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-ready:
		if line != "exec-ready" {
			t.Fatalf("exec descriptor check failed, output %q, stderr %q", line, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("gate did not execute after lock release, stderr %q", stderr.String())
	}

	contender, err := os.OpenFile(lockPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer contender.Close()
	if err := syscall.Flock(int(contender.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		t.Fatal("lock was not preserved across exec")
	} else if !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("unexpected lock contention error: %v", err)
	}

	if _, err := stdinWriter.Write([]byte("exit\n")); err != nil {
		t.Fatal(err)
	}
	if err := stdinWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("executed command failed: %v; stderr %q", err, stderr.String())
	}
	if err := syscall.Flock(int(contender.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("lock was not released after process exit: %v", err)
	}
}

func TestTerminatedWaitingGateHasNoLivePreparedDescriptor(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "agent.lock")
	preparedPath := filepath.Join(dir, "agent.prepared")

	oldLock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer oldLock.Close()
	if err := syscall.Flock(int(oldLock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0],
		"-test.run=^TestGateHelperProcess$", "--",
		"--component", "agent", "--", "true",
	)
	command.Env = append(os.Environ(),
		gateHelperEnv+"=1",
		lockPathEnv+"="+lockPath,
		preparedPathEnv+"="+preparedPath,
		podUIDEnv+"=replacement-pod",
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFileContents(t, preparedPath)

	contents, err := os.ReadFile(preparedPath)
	if err != nil {
		t.Fatal(err)
	}
	var uid string
	var pid, fd int
	if _, err := fmt.Sscanf(string(contents), "%s %d %d", &uid, &pid, &fd); err != nil {
		t.Fatal(err)
	}
	if uid != "replacement-pod" || pid != command.Process.Pid {
		t.Fatalf("unexpected Prepared marker %q", contents)
	}

	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("waiting gate did not exit after SIGTERM")
	}
	if _, err := os.Stat(fmt.Sprintf("/proc/%d/fd/%d", pid, fd)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Prepared descriptor remained live after waiting gate SIGTERM: %v", err)
	}
}

func waitForFileContents(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil && len(contents) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not become non-empty", path)
}
