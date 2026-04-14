// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

// Package impl — IPC protocol tests.
//
// These tests verify the HTTP/1.1 over UDS wire protocol between par-control
// and par-executor without the full FX stack.  They test the HTTP handler
// layer directly, confirming that:
//   - The request/response JSON field names and types are correct.
//   - base64.StdEncoding decoding works for the raw_task field.
//   - error_code=0 is returned for successful actions.
//   - Non-zero error_code is returned for allowlist violations.
//   - 400 is returned for malformed requests.
package impl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-go/v5/statsd"

	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	credresolver "github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	com_datadoghq_remoteaction "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction"
)

// testServer starts the executor HTTP handler on a temp Unix socket and
// returns the socket path plus a cleanup function.
func testServer(t *testing.T, allowlist map[string]sets.Set[string]) (socketPath string, stop func()) {
	t.Helper()
	t.Setenv("DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION", "true")

	cfg := &parconfig.Config{
		ActionsAllowlist: allowlist,
		MetricsClient:    &statsd.NoOpClient{},
	}

	var verifier *taskverifier.TaskVerifier // nil — skip path in unwrapTask

	resolver := credresolver.NewPrivateCredentialResolver()

	bundles := map[string]types.Bundle{
		"com.datadoghq.remoteaction": com_datadoghq_remoteaction.NewRemoteAction(),
	}

	ctx := context.Background()
	var lastActivity atomic.Int64
	handler := makeExecuteHandler(ctx, cfg, verifier, resolver, bundles, &lastActivity, 0)

	// Use /tmp with a short name — t.TempDir() produces paths that exceed
	// macOS's 104-character Unix socket path limit.
	f, err := os.CreateTemp("/tmp", "pex*.sock")
	require.NoError(t, err)
	socketPath = f.Name()
	f.Close()
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	os.Chmod(socketPath, 0720) //nolint:errcheck

	mux := http.NewServeMux()
	mux.HandleFunc("/execute", handler)
	mux.HandleFunc("/debug/ready", handleReady)
	mux.HandleFunc("/debug/health", handleHealth)

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln) //nolint:errcheck

	return socketPath, func() {
		srv.Close()
		ln.Close()
		os.Remove(socketPath)
	}
}

// udsClient builds an http.Client that dials a Unix socket.
// Mirrors pkg/system-probe/api/client/client_unix.go.
func udsClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

// makeRawTask builds the base64-encoded raw task bytes that par-control
// would forward from OPMS to /execute.
func makeRawTask(t *testing.T, bundleID, actionName string, inputs map[string]interface{}) string {
	t.Helper()

	inputStruct, err := structpb.NewStruct(inputs)
	require.NoError(t, err)

	pbTask := &privateactionspb.PrivateActionTask{
		TaskId:     "test-task-001",
		BundleId:   bundleID,
		ActionName: actionName,
		OrgId:      12345,
		Inputs:     inputStruct,
	}
	pbBytes, err := proto.Marshal(pbTask)
	require.NoError(t, err)

	envelope := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     pbBytes,
		HashType: privateactionspb.HashType_SHA256,
		// No signatures — skip verification bypasses the signature check.
	}

	var task types.Task
	task.Data.ID = "test-task-001"
	task.Data.Type = "workflowTask"
	task.Data.Attributes = &types.Attributes{
		Name:           actionName,
		BundleID:       bundleID,
		JobId:          "job-001",
		OrgId:          12345,
		SignedEnvelope: envelope,
	}

	raw, err := json.Marshal(task)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(raw)
}

// post calls POST /execute and decodes the ExecuteResponse.
func post(t *testing.T, client *http.Client, socketPath, rawTaskB64 string) (int, ExecuteResponse) {
	t.Helper()

	timeout := int32(10)
	req := ExecuteRequest{RawTask: rawTaskB64, TimeoutSeconds: &timeout}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := client.Post("http://par-executor/execute", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result ExecuteResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return resp.StatusCode, result
}

// ── Tests ─────────────────────────────────────────────────────────────────

func TestIPC_DebugEndpoints(t *testing.T) {
	socketPath, stop := testServer(t, map[string]sets.Set[string]{})
	defer stop()

	client := udsClient(socketPath)

	resp, err := client.Get("http://par-executor/debug/ready")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	resp, err = client.Get("http://par-executor/debug/health")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
}

// TestIPC_Execute_TestConnection verifies the happy path: par-control sends a
// testConnection task, executor returns success with output.
func TestIPC_Execute_TestConnection(t *testing.T) {
	allowlist := map[string]sets.Set[string]{
		"com.datadoghq.remoteaction": sets.New[string]("testConnection"),
	}
	socketPath, stop := testServer(t, allowlist)
	defer stop()

	rawTaskB64 := makeRawTask(t, "com.datadoghq.remoteaction", "testConnection", nil)
	status, result := post(t, udsClient(socketPath), socketPath, rawTaskB64)

	assert.Equal(t, 200, status)
	assert.Equal(t, int32(0), result.ErrorCode, "expected success: %s", result.ErrorDetails)
	require.NotNil(t, result.Output)

	// testConnection output: {"success": true, "agentInfo": {"version": "..."}}
	out := result.Output.(map[string]interface{})
	assert.Equal(t, true, out["success"])
}

// TestIPC_Execute_AllowlistBlocked verifies that an action not in the
// allowlist returns a non-zero error_code.
func TestIPC_Execute_AllowlistBlocked(t *testing.T) {
	// Allow testConnection but not runCommand.
	allowlist := map[string]sets.Set[string]{
		"com.datadoghq.remoteaction": sets.New[string]("testConnection"),
	}
	socketPath, stop := testServer(t, allowlist)
	defer stop()

	// rshell.runCommand is not registered and not in allowlist.
	rawTaskB64 := makeRawTask(t, "com.datadoghq.remoteaction", "unknownAction", nil)
	_, result := post(t, udsClient(socketPath), socketPath, rawTaskB64)

	assert.NotEqual(t, int32(0), result.ErrorCode)
}

// TestIPC_Execute_BadBase64 verifies that malformed base64 in raw_task
// returns HTTP 400, consistent with the error handling the Rust client expects.
func TestIPC_Execute_BadBase64(t *testing.T) {
	socketPath, stop := testServer(t, map[string]sets.Set[string]{})
	defer stop()

	body := `{"raw_task": "!!not-valid-base64!!"}`
	resp, err := udsClient(socketPath).Post(
		"http://par-executor/execute",
		"application/json",
		bytes.NewReader([]byte(body)),
	)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ensure log adapter used (avoids unused import errors)
var _ = log.FromContext
