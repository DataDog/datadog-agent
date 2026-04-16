// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

// Package impl — end-to-end subprocess tests.
//
// These tests exercise the complete dual-process flow as it would run in
// production: par-executor is started as a real OS subprocess, tasks are
// dispatched to it via the binary UDS protocol (the same channel par-control
// uses), and results are checked.
//
// Build the binary once with TestMain and share it across subtests.
package impl

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	if runtime.GOOS != "linux" {
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

func buildParExecutor() (string, error) {
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

// startExecutor launches par-executor as a subprocess, waits for ping readiness.
func startExecutor(t *testing.T, allowedFQNs []string, idleTimeoutSec int) (socketPath string, cleanup func()) {
	t.Helper()
	if e2eBin == "" {
		t.Skip("par-executor binary not built (non-Linux host)")
	}

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

	require.NoError(t, waitPing(socketPath, 10*time.Second),
		"par-executor did not become ready within 10s")

	return socketPath, func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		os.Remove(socketPath)
	}
}

// waitPing polls the ping frame until pong is received or deadline.
func waitPing(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", socketPath, time.Second)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		conn.Write([]byte{framePing}) //nolint:errcheck
		var pong [1]byte
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, readErr := io.ReadFull(conn, pong[:])
		conn.Close()
		if readErr == nil && pong[0] == 0x01 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for ping pong on %s", socketPath)
}

// callExecuteE2E dispatches a task to the subprocess executor via binary protocol.
// Returns (status byte, decoded output or nil, error details).
func callExecuteE2E(t *testing.T, socketPath string, rawTask []byte, timeoutSecs uint32) (byte, map[string]interface{}, string) {
	t.Helper()

	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	conn.Write([]byte{frameExecute})                                     //nolint:errcheck
	binary.Write(conn, binary.LittleEndian, uint32(len(rawTask)))        //nolint:errcheck
	conn.Write(rawTask)                                                   //nolint:errcheck
	binary.Write(conn, binary.LittleEndian, timeoutSecs)                 //nolint:errcheck

	status, payload, readErr := readResponse(conn)
	require.NoError(t, readErr)

	var output map[string]interface{}
	var errDetails string
	if status == statusOK {
		json.Unmarshal(payload, &output) //nolint:errcheck
	} else {
		var errResp errorResponse
		json.Unmarshal(payload, &errResp) //nolint:errcheck
		errDetails = errResp.ErrorDetails
	}
	return status, output, errDetails
}

// makeE2ETask builds raw OPMS-format task JSON for the given bundle/action.
func makeE2ETask(t *testing.T, bundleID, actionName string, inputs map[string]interface{}) []byte {
	t.Helper()
	inputStruct, err := structpb.NewStruct(inputs)
	require.NoError(t, err)

	pbTask := &privateactionspb.PrivateActionTask{
		TaskId: "e2e-001", BundleId: bundleID, ActionName: actionName,
		OrgId: 12345, Inputs: inputStruct,
	}
	pbBytes, err := proto.Marshal(pbTask)
	require.NoError(t, err)

	var task types.Task
	task.Data.ID = "e2e-001"
	task.Data.Type = "workflowTask"
	task.Data.Attributes = &types.Attributes{
		Name: actionName, BundleID: bundleID, JobId: "e2e-job", OrgId: 12345,
		SignedEnvelope: &privateactionspb.RemoteConfigSignatureEnvelope{
			Data: pbBytes, HashType: privateactionspb.HashType_SHA256,
		},
	}
	raw, err := json.Marshal(task)
	require.NoError(t, err)
	return raw
}

// ── E2E tests ─────────────────────────────────────────────────────────────

func TestE2E_FullLifecycle_TestConnection(t *testing.T) {
	socketPath, stop := startExecutor(t, []string{"com.datadoghq.remoteaction.testConnection"}, 120)
	defer stop()

	raw := makeE2ETask(t, "com.datadoghq.remoteaction", "testConnection", nil)
	status, output, errDetails := callExecuteE2E(t, socketPath, raw, 30)

	require.Equal(t, statusOK, status, "testConnection failed: %s", errDetails)
	require.NotNil(t, output)
	assert.Equal(t, true, output["success"])
	t.Logf("testConnection output: %v", output)
}

func TestE2E_FullLifecycle_RShellRunCommand(t *testing.T) {
	socketPath, stop := startExecutor(t, []string{"com.datadoghq.remoteaction.rshell.runCommand"}, 120)
	defer stop()

	raw := makeE2ETask(t, "com.datadoghq.remoteaction.rshell", "runCommand", map[string]interface{}{
		"command":         "echo hello",
		"allowedCommands": []interface{}{"echo"},
	})
	status, output, errDetails := callExecuteE2E(t, socketPath, raw, 30)

	require.Equal(t, statusOK, status, "runCommand failed: %s", errDetails)
	require.NotNil(t, output)
	assert.Equal(t, float64(0), output["exitCode"])
	assert.Equal(t, "hello\n", output["stdout"])
}

func TestE2E_IdleTimeout(t *testing.T) {
	const idleTimeoutSec = 3

	socketPath, stop := startExecutor(t, []string{"com.datadoghq.remoteaction.testConnection"}, idleTimeoutSec)
	defer stop()

	// Confirm executor is working.
	raw := makeE2ETask(t, "com.datadoghq.remoteaction", "testConnection", nil)
	status, _, _ := callExecuteE2E(t, socketPath, raw, 10)
	require.Equal(t, statusOK, status)

	// Wait for idle timer — executor should stop responding to pings.
	deadline := time.Now().Add(time.Duration(idleTimeoutSec+10) * time.Second)
	exited := false
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
		if err != nil {
			exited = true
			break
		}
		conn.Write([]byte{framePing}) //nolint:errcheck
		var pong [1]byte
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if _, err := io.ReadFull(conn, pong[:]); err != nil {
			conn.Close()
			exited = true
			break
		}
		conn.Close()
		time.Sleep(500 * time.Millisecond)
	}
	assert.True(t, exited, "par-executor should have self-terminated after %ds idle", idleTimeoutSec)
}

func TestE2E_ConcurrentActions(t *testing.T) {
	socketPath, stop := startExecutor(t, []string{"com.datadoghq.remoteaction.testConnection"}, 120)
	defer stop()

	const n = 5
	type result struct{ status byte }
	results := make(chan result, n)
	raw := makeE2ETask(t, "com.datadoghq.remoteaction", "testConnection", nil)

	for i := 0; i < n; i++ {
		go func() {
			s, _, _ := callExecuteE2E(t, socketPath, raw, 30)
			results <- result{s}
		}()
	}
	for i := 0; i < n; i++ {
		r := <-results
		assert.Equal(t, statusOK, r.status, "concurrent action %d failed", i)
	}
}
