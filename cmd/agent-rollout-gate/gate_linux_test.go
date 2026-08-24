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
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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

func TestWriteLiveMarkerIdentifiesTheOpenDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "prepared")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := writeLiveMarker(file, "pod-uid"); err != nil {
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

func TestMissingCommandNeverAdvertisesPrepared(t *testing.T) {
	dir := t.TempDir()
	opts := options{
		component:    "agent",
		lockPath:     dir + "/agent.lock",
		preparedPath: dir + "/agent.prepared",
		activePath:   dir + "/agent.active",
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

	err = waitAndExec(opts, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "resolve command before advertising Prepared") {
		t.Fatalf("got %v, want pre-Prepared command error", err)
	}
	if _, err := os.Stat(preparedMarkerPath(opts.preparedPath, opts.podUID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Prepared marker exists for a missing command: %v", err)
	}
	if _, err := os.Stat(opts.activePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Active marker exists for a missing command: %v", err)
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

func TestLockAndActiveMarkerDescriptorSurviveExec(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "agent.lock")
	preparedPath := filepath.Join(dir, "agent.prepared")
	preparedPodPath := preparedMarkerPath(preparedPath, "replacement-pod")
	activePath := filepath.Join(dir, "agent.active")

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
		"--component", "agent", "--state-dir", dir, "--", "sh", "-c",
		`test ! -e "$TEST_PREPARED_PATH" && test -z "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID" && test -z "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_IP" && read uid pid fd < "$TEST_ACTIVE_PATH" && test "$pid" = "$$" && test "/proc/$$/fd/$fd" -ef "$TEST_ACTIVE_PATH" && echo exec-ready && read line`,
	)
	command.Env = append(os.Environ(),
		gateHelperEnv+"=1",
		podUIDEnv+"=replacement-pod",
		podIPEnv+"=192.0.2.10",
		"TEST_PREPARED_PATH="+preparedPodPath,
		"TEST_ACTIVE_PATH="+activePath,
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

	waitForFileContents(t, preparedPodPath)
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
	preparedPodPath := preparedMarkerPath(preparedPath, "replacement-pod")
	activePath := filepath.Join(dir, "agent.active")

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
		"--component", "agent", "--state-dir", dir, "--", "true",
	)
	command.Env = append(os.Environ(),
		gateHelperEnv+"=1",
		podUIDEnv+"=replacement-pod",
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFileContents(t, preparedPodPath)

	contents, err := os.ReadFile(preparedPodPath)
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
	if _, err := os.Stat(activePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Active marker was created before lock acquisition: %v", err)
	}
}

func TestStartupProbeFailsWhilePreparedAndPassesWhenActive(t *testing.T) {
	dir := t.TempDir()
	preparedPath := filepath.Join(dir, "agent.prepared")
	activePath := filepath.Join(dir, "agent.active")
	prepared := createLiveMarker(t, preparedMarkerPath(preparedPath, "pod-uid"), "pod-uid")
	defer prepared.Close()

	base := probeOptions{address: "127.0.0.1", preparedPath: preparedPath, activePath: activePath, podUID: "pod-uid", timeout: 100 * time.Millisecond}
	startup := base
	startup.kind = "startup"
	startup.handler = "active"
	if err := runProbe(startup); err == nil {
		t.Fatal("Prepared container passed startup before Active")
	}

	active := createLiveMarker(t, activePath, "pod-uid")
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(preparedMarkerPath(preparedPath, "pod-uid")); err != nil {
		t.Fatal(err)
	}
	if err := runProbe(startup); err != nil {
		t.Fatalf("Active startup failed: %v", err)
	}

	if err := active.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runProbe(startup); err == nil {
		t.Fatal("startup accepted a stale Active marker")
	}
}

func TestStartupAcceptsActiveAfterPreparedMarkerIsReplaced(t *testing.T) {
	dir := t.TempDir()
	preparedPath := filepath.Join(dir, "agent.prepared")
	activePath := filepath.Join(dir, "agent.active")
	prepared := createLiveMarker(t, preparedMarkerPath(preparedPath, "old-pod"), "old-pod")
	defer prepared.Close()
	active := createLiveMarker(t, activePath, "replacement-pod")
	defer active.Close()

	startup := probeOptions{
		kind:         "startup",
		handler:      "active",
		timeout:      100 * time.Millisecond,
		preparedPath: preparedPath,
		activePath:   activePath,
		podUID:       "replacement-pod",
	}
	if err := runProbe(startup); err != nil {
		t.Fatalf("Active startup failed after another Pod replaced Prepared marker: %v", err)
	}
}

func TestActiveProbeDelegatesHTTPAndTCPHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/bad" {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	activePath := filepath.Join(dir, "agent.active")
	active := createLiveMarker(t, activePath, "pod-uid")
	defer active.Close()
	base := probeOptions{kind: "startup", handler: "http", address: "127.0.0.1", port: port, path: "/ready", timeout: time.Second, preparedPath: filepath.Join(dir, "agent.prepared"), activePath: activePath, podUID: "pod-uid"}
	if err := runProbe(base); err != nil {
		t.Fatalf("HTTP redirect status should be healthy: %v", err)
	}
	base.path = "/bad"
	if err := runProbe(base); err == nil {
		t.Fatal("HTTP 503 status reported healthy")
	}
	base.handler = "tcp"
	base.path = ""
	if err := runProbe(base); err != nil {
		t.Fatalf("TCP listener reported unhealthy: %v", err)
	}
}

func TestActiveStartupFailureBudgetTerminatesThenKills(t *testing.T) {
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if healthy.Load() {
			writer.WriteHeader(http.StatusOK)
			return
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	activePath := filepath.Join(dir, "agent.active")
	active := createLiveMarker(t, activePath, "pod-uid")
	defer active.Close()
	opts := probeOptions{
		kind: "startup", handler: "http", address: "127.0.0.1", port: port, path: "/ready",
		timeout: time.Second, failureThreshold: 2, terminationGracePeriod: time.Millisecond,
		preparedPath: filepath.Join(dir, "agent.prepared"), activePath: activePath, podUID: "pod-uid",
	}

	originalSignal := signalActiveProcess
	defer func() { signalActiveProcess = originalSignal }()
	var signals []syscall.Signal
	signalActiveProcess = func(pid int, signal syscall.Signal) error {
		if pid != os.Getpid() {
			t.Fatalf("signaled pid %d, want %d", pid, os.Getpid())
		}
		signals = append(signals, signal)
		return nil
	}

	if err := runProbe(opts); err == nil {
		t.Fatal("first unhealthy startup probe passed")
	}
	if len(signals) != 0 {
		t.Fatalf("signaled before the original failure threshold: %v", signals)
	}
	if err := runProbe(opts); err == nil {
		t.Fatal("second unhealthy startup probe passed")
	}
	if len(signals) != 1 || signals[0] != syscall.SIGTERM {
		t.Fatalf("signals after threshold = %v, want [SIGTERM]", signals)
	}
	healthy.Store(true)
	time.Sleep(2 * time.Millisecond)
	if err := runProbe(opts); err == nil {
		t.Fatal("startup passed after a requested restart merely because health recovered")
	}
	if len(signals) != 2 || signals[1] != syscall.SIGKILL {
		t.Fatalf("signals after grace = %v, want SIGKILL escalation", signals)
	}
}

func TestActiveStartupBookkeepingFailureTerminatesProcess(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "agent.active")
	active := createLiveMarker(t, activePath, "pod-uid")
	defer active.Close()
	opts := probeOptions{
		kind: "startup", handler: "tcp", address: "127.0.0.1", port: unusedTCPPort(t),
		timeout: 10 * time.Millisecond, failureThreshold: 3, terminationGracePeriod: time.Second,
		preparedPath: filepath.Join(dir, "agent.prepared"), activePath: activePath, podUID: "pod-uid",
	}
	if err := os.Mkdir(startupFailurePath(opts.activePath, opts.podUID), 0o700); err != nil {
		t.Fatal(err)
	}

	originalSignal := signalActiveProcess
	defer func() { signalActiveProcess = originalSignal }()
	var signals []syscall.Signal
	signalActiveProcess = func(pid int, signal syscall.Signal) error {
		if pid != os.Getpid() {
			t.Fatalf("signaled pid %d, want %d", pid, os.Getpid())
		}
		signals = append(signals, signal)
		return nil
	}

	err := runProbe(opts)
	if err == nil || !strings.Contains(err.Error(), "bookkeeping failed") {
		t.Fatalf("startup bookkeeping failure = %v, want explicit error", err)
	}
	if len(signals) != 1 || signals[0] != syscall.SIGTERM {
		t.Fatalf("signals after bookkeeping failure = %v, want [SIGTERM]", signals)
	}
}

func TestActiveStartupSuccessResetsConsecutiveFailureBudget(t *testing.T) {
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if !healthy.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	activePath := filepath.Join(dir, "agent.active")
	active := createLiveMarker(t, activePath, "pod-uid")
	defer active.Close()
	opts := probeOptions{
		kind: "startup", handler: "http", address: "127.0.0.1", port: port, path: "/ready",
		timeout: time.Second, failureThreshold: 2, terminationGracePeriod: time.Second,
		preparedPath: filepath.Join(dir, "agent.prepared"), activePath: activePath, podUID: "pod-uid",
	}

	originalSignal := signalActiveProcess
	defer func() { signalActiveProcess = originalSignal }()
	var signalCount int
	signalActiveProcess = func(int, syscall.Signal) error {
		signalCount++
		return nil
	}

	if err := runProbe(opts); err == nil {
		t.Fatal("unhealthy startup probe passed")
	}
	healthy.Store(true)
	if err := runProbe(opts); err != nil {
		t.Fatalf("healthy startup probe failed: %v", err)
	}
	healthy.Store(false)
	if err := runProbe(opts); err == nil {
		t.Fatal("unhealthy startup probe passed after recovery")
	}
	if signalCount != 0 {
		t.Fatalf("recovered probe did not reset the consecutive failure budget; got %d signals", signalCount)
	}
}

func TestPreparedStartupFailuresDoNotConsumeActiveBudget(t *testing.T) {
	dir := t.TempDir()
	opts := probeOptions{
		kind: "startup", handler: "tcp", address: "127.0.0.1", port: unusedTCPPort(t),
		timeout: 10 * time.Millisecond, failureThreshold: 1, terminationGracePeriod: time.Second,
		preparedPath: filepath.Join(dir, "agent.prepared"), activePath: filepath.Join(dir, "agent.active"), podUID: "pod-uid",
	}
	prepared := createLiveMarker(t, preparedMarkerPath(opts.preparedPath, opts.podUID), opts.podUID)
	defer prepared.Close()

	originalSignal := signalActiveProcess
	defer func() { signalActiveProcess = originalSignal }()
	var signalCount int
	signalActiveProcess = func(int, syscall.Signal) error {
		signalCount++
		return nil
	}
	for i := 0; i < 3; i++ {
		if err := runProbe(opts); err == nil {
			t.Fatal("Prepared startup probe passed")
		}
	}
	if signalCount != 0 {
		t.Fatalf("Prepared failures signaled an Agent process %d times", signalCount)
	}
	if _, err := os.Stat(startupFailurePath(opts.activePath, opts.podUID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Prepared failures created Active failure state: %v", err)
	}
}

func TestMarkerOwnershipRejectsWrongPodAndReplacedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.active")
	marker := createLiveMarker(t, path, "pod-a")
	defer marker.Close()
	if markerOwnedByPod(path, "pod-b") {
		t.Fatal("marker from another Pod was accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("pod-a %d %d\n", os.Getpid(), marker.Fd())), 0o644); err != nil {
		t.Fatal(err)
	}
	if markerOwnedByPod(path, "pod-a") {
		t.Fatal("replacement path with a different inode was accepted")
	}
}

func TestPreparedMarkersArePodSpecific(t *testing.T) {
	basePath := filepath.Join(t.TempDir(), "agent.prepared")
	podAPath := preparedMarkerPath(basePath, "pod-a")
	podBPath := preparedMarkerPath(basePath, "pod-b")
	requireDifferent := func(left, right string) {
		t.Helper()
		if left == right {
			t.Fatalf("prepared marker paths must differ: %q", left)
		}
	}
	requireDifferent(podAPath, podBPath)
	podA := createLiveMarker(t, podAPath, "pod-a")
	defer podA.Close()
	podB := createLiveMarker(t, podBPath, "pod-b")
	defer podB.Close()

	if !markerOwnedByPod(podAPath, "pod-a") || !markerOwnedByPod(podBPath, "pod-b") {
		t.Fatal("one Pod's Prepared marker invalidated the other Pod's lease")
	}
}

func createLiveMarker(t *testing.T, path, podUID string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLiveMarker(file, podUID); err != nil {
		file.Close()
		t.Fatal(err)
	}
	return file
}

func unusedTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
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
