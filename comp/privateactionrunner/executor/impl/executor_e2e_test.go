// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

// Package impl — end-to-end subprocess tests.
//
// These tests exercise the complete dual-process flow as it would run in
// production: par-executor is started as a real OS subprocess, tasks are
// dispatched to it via HTTP/1.1 over UDS (the same channel par-control uses),
// and results are checked.  They validate the full lifecycle:
//
//   par-control equivalent (test) → fork+exec par-executor → /debug/ready
//   → POST /execute (testConnection, rshell.runCommand) → verify output
//   → idle timeout → process exits
//
// Build the binary once with TestMain and share it across subtests.
package impl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// e2eBin is the path to the par-executor binary built during TestMain.
var e2eBin string

// TestMain builds par-executor once and shares it across all e2e tests.
func TestMain(m *testing.M) {
	// Only build if running on Linux — the executor's UDS server and rshell
	// are Linux-specific.  On macOS the IPC tests (executor_ipc_test.go) are
	// sufficient; the subprocess tests require a Linux environment.
	if runtime.GOOS != "linux" {
		// Still run non-subprocess tests (IPC tests live in the same package).
		os.Exit(m.Run())
	}

	bin, err := buildParExecutor()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to build par-executor: %v\n", err)
		os.Exit(1)
	}
	e2eBin = bin
	os.Exit(m.Run())
}

// ── Subprocess helpers ────────────────────────────────────────────────────

func buildParExecutor() (string, error) {
	// Locate the repo root by walking up from this file's directory.
	_, thisFile, _, _ := runtime.Caller(0)
	dir := thisFile
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root (go.mod)")
		}
		dir = parent
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
	}

	bin := filepath.Join(os.TempDir(), "par-executor-e2e")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/par-executor/")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build failed: %w\n%s", err, out)
	}
	return bin, nil
}

// startExecutor launches par-executor as a subprocess and waits for it to
// signal readiness via GET /debug/ready.  Returns the socket path and a
// cleanup function that kills the process.
//
// allowedFQNs is set via the DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST env
// var, which the DD config system picks up automatically.
func startExecutor(t *testing.T, allowedFQNs []string, idleTimeoutSec int) (socketPath string, cleanup func()) {
	t.Helper()
	if e2eBin == "" {
		t.Skip("par-executor binary not built (non-Linux host)")
	}

	// Short socket path to stay within the 104-char macOS/Linux UDS limit.
	f, err := os.CreateTemp("/tmp", "pexe2e*.sock")
	require.NoError(t, err)
	socketPath = f.Name()
	f.Close()
	os.Remove(socketPath)

	allowlist := ""
	for i, fqn := range allowedFQNs {
		if i > 0 {
			allowlist += ","
		}
		allowlist += fqn
	}

	env := append(os.Environ(),
		"DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true",
		"DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST="+allowlist,
		"DD_PRIVATE_ACTION_RUNNER_ENABLED=true",
	)

	cmd := exec.Command(e2eBin, "run",
		"--socket", socketPath,
		"--idle-timeout-seconds", fmt.Sprintf("%d", idleTimeoutSec),
	)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	// Poll GET /debug/ready — same as par-control STARTING state.
	require.NoError(t, waitReady(socketPath, 10*time.Second),
		"par-executor did not become ready within 10s")

	return socketPath, func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		os.Remove(socketPath)
	}
}

// waitReady polls GET /debug/ready until 200 OK or deadline.
func waitReady(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := udsHTTPClient(socketPath)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://par-executor/debug/ready")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for /debug/ready on %s", socketPath)
}

// udsHTTPClient mirrors pkg/system-probe/api/client/client_unix.go.
func udsHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

// callExecute builds and fires POST /execute.  Returns (httpStatus, response).
func callExecute(t *testing.T, socketPath, rawTaskB64 string, timeoutSec int32) (int, ExecuteResponse) {
	t.Helper()
	req := ExecuteRequest{RawTask: rawTaskB64, TimeoutSeconds: &timeoutSec}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := udsHTTPClient(socketPath).Post(
		"http://par-executor/execute",
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result ExecuteResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return resp.StatusCode, result
}

// makeTask builds the base64-encoded raw task bytes that par-control would
// forward to /execute — mirrors the OPMS dequeue response format.
func makeTask(t *testing.T, bundleID, actionName string, inputs map[string]interface{}) string {
	t.Helper()
	inputStruct, err := structpb.NewStruct(inputs)
	require.NoError(t, err)

	pbTask := &privateactionspb.PrivateActionTask{
		TaskId:     "e2e-task-001",
		BundleId:   bundleID,
		ActionName: actionName,
		OrgId:      12345,
		Inputs:     inputStruct,
	}
	pbBytes, err := proto.Marshal(pbTask)
	require.NoError(t, err)

	var task types.Task
	task.Data.ID = "e2e-task-001"
	task.Data.Type = "workflowTask"
	task.Data.Attributes = &types.Attributes{
		Name:    actionName,
		BundleID: bundleID,
		JobId:   "e2e-job-001",
		OrgId:   12345,
		SignedEnvelope: &privateactionspb.RemoteConfigSignatureEnvelope{
			Data:     pbBytes,
			HashType: privateactionspb.HashType_SHA256,
		},
	}
	raw, err := json.Marshal(task)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(raw)
}

// ── E2E tests ─────────────────────────────────────────────────────────────

// TestE2E_FullLifecycle_TestConnection validates the core dual-process flow:
// par-control analogue → executor subprocess → /execute → testConnection output.
func TestE2E_FullLifecycle_TestConnection(t *testing.T) {
	socketPath, stop := startExecutor(t, []string{
		"com.datadoghq.remoteaction.testConnection",
	}, 120)
	defer stop()

	rawTask := makeTask(t, "com.datadoghq.remoteaction", "testConnection", nil)
	status, resp := callExecute(t, socketPath, rawTask, 30)

	assert.Equal(t, 200, status)
	require.Equal(t, int32(0), resp.ErrorCode, "testConnection failed: %s", resp.ErrorDetails)
	require.NotNil(t, resp.Output)

	out, ok := resp.Output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, out["success"])
	assert.NotNil(t, out["agentInfo"])
	t.Logf("testConnection output: %v", out)
}

// TestE2E_FullLifecycle_RShellRunCommand validates rshell action execution:
// command="echo hello" should return exitCode=0, stdout="hello\n".
func TestE2E_FullLifecycle_RShellRunCommand(t *testing.T) {
	socketPath, stop := startExecutor(t, []string{
		"com.datadoghq.remoteaction.rshell.runCommand",
	}, 120)
	defer stop()

	rawTask := makeTask(t, "com.datadoghq.remoteaction.rshell", "runCommand", map[string]interface{}{
		"command":         "echo hello",
		"allowedCommands": []interface{}{"echo"},
	})
	status, resp := callExecute(t, socketPath, rawTask, 30)

	assert.Equal(t, 200, status)
	require.Equal(t, int32(0), resp.ErrorCode, "runCommand failed: %s", resp.ErrorDetails)
	require.NotNil(t, resp.Output)

	out, ok := resp.Output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(0), out["exitCode"])
	assert.Equal(t, "hello\n", out["stdout"])
	t.Logf("runCommand output: %v", out)
}

// TestE2E_IdleTimeout verifies that par-executor self-terminates after the
// configured idle period with no active requests.
func TestE2E_IdleTimeout(t *testing.T) {
	const idleTimeoutSec = 3 // short timeout for test speed

	socketPath, stop := startExecutor(t, []string{
		"com.datadoghq.remoteaction.testConnection",
	}, idleTimeoutSec)
	defer stop() // belt-and-suspenders cleanup

	// Run one action to confirm the executor is working.
	rawTask := makeTask(t, "com.datadoghq.remoteaction", "testConnection", nil)
	_, resp := callExecute(t, socketPath, rawTask, 10)
	require.Equal(t, int32(0), resp.ErrorCode)

	// Wait for the idle timer to fire (idleTimeoutSec + 2s watcher tick + margin).
	deadline := time.Now().Add(time.Duration(idleTimeoutSec+10) * time.Second)
	exited := false
	for time.Now().Before(deadline) {
		// Health ping: if the executor has exited, the connection will fail.
		resp, err := udsHTTPClient(socketPath).Get("http://par-executor/debug/health")
		if err != nil {
			exited = true
			break
		}
		resp.Body.Close()
		time.Sleep(500 * time.Millisecond)
	}
	assert.True(t, exited, "par-executor should have self-terminated after %ds idle", idleTimeoutSec)
}

// TestE2E_ConcurrentActions verifies that multiple actions can run in parallel
// (up to RunnerPoolSize) — the semaphore allows concurrent dispatch.
func TestE2E_ConcurrentActions(t *testing.T) {
	socketPath, stop := startExecutor(t, []string{
		"com.datadoghq.remoteaction.testConnection",
	}, 120)
	defer stop()

	const n = 5
	results := make(chan ExecuteResponse, n)
	rawTask := makeTask(t, "com.datadoghq.remoteaction", "testConnection", nil)

	// Fire n requests concurrently — they should all complete successfully.
	for i := 0; i < n; i++ {
		go func() {
			_, resp := callExecute(t, socketPath, rawTask, 30)
			results <- resp
		}()
	}

	for i := 0; i < n; i++ {
		resp := <-results
		assert.Equal(t, int32(0), resp.ErrorCode, "concurrent action %d failed: %s", i, resp.ErrorDetails)
	}
}
