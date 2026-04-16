// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

// Package impl — IPC protocol tests.
//
// Tests verify the binary length-framing protocol between par-control and
// par-executor without the full FX stack, by exercising the binary handler
// layer directly:
//   - Ping frames return a pong
//   - Execute frames with a valid task return success + correct output
//   - Allowlist violations return a non-zero error_code
//   - Malformed requests (zero-length task) return an error gracefully
//   - SO_PEERCRED: connection from the same process passes (in-process test)
package impl

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"sync"
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
	com_datadoghq_remoteaction "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction"
	credresolver "github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// testServer starts the executor binary-protocol handler on a temp Unix socket.
func testServer(t *testing.T, allowlist map[string]sets.Set[string]) (socketPath string, stop func()) {
	t.Helper()
	t.Setenv("DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION", "true")

	cfg := &parconfig.Config{
		ActionsAllowlist: allowlist,
		MetricsClient:    &statsd.NoOpClient{},
	}
	// DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true is set above, so
	// NewTaskVerifier returns the no-op verifier (no RC connection needed).
	verifier := taskverifier.NewTaskVerifier(nil, cfg)
	resolver := credresolver.NewPrivateCredentialResolver()
	bundles := map[string]types.Bundle{
		"com.datadoghq.remoteaction": com_datadoghq_remoteaction.NewRemoteAction(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	f, err := os.CreateTemp("/tmp", "pex*.sock")
	require.NoError(t, err)
	socketPath = f.Name()
	f.Close()
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	os.Chmod(socketPath, 0720) //nolint:errcheck

	ec := &ExecutorComponent{
		params: Params{IdleTimeoutSeconds: 0},
	}

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				ec.handleConn(ctx, conn, cfg, verifier, resolver, bundles, sem)
			}()
		}
	}()

	return socketPath, func() {
		cancel()
		ln.Close()
		wg.Wait()
		os.Remove(socketPath)
	}
}

// dialUDS opens a connection to the test server.
func dialUDS(t *testing.T, socketPath string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	require.NoError(t, err)
	return conn
}

// sendPing sends a ping frame and reads the pong byte.
func sendPing(t *testing.T, socketPath string) bool {
	t.Helper()
	conn := dialUDS(t, socketPath)
	defer conn.Close()
	_, err := conn.Write([]byte{framePing})
	require.NoError(t, err)
	var pong [1]byte
	_, err = io.ReadFull(conn, pong[:])
	require.NoError(t, err)
	return pong[0] == 0x01
}

// sendExecute sends a binary execute frame and reads the response.
func sendExecute(t *testing.T, socketPath string, rawTask []byte, timeoutSecs uint32) (status byte, payload []byte) {
	t.Helper()
	conn := dialUDS(t, socketPath)
	defer conn.Close()

	// Write: [frame_type][task_len][task bytes][timeout]
	_, err := conn.Write([]byte{frameExecute})
	require.NoError(t, err)
	require.NoError(t, binary.Write(conn, binary.LittleEndian, uint32(len(rawTask))))
	_, err = conn.Write(rawTask)
	require.NoError(t, err)
	require.NoError(t, binary.Write(conn, binary.LittleEndian, timeoutSecs))

	s, p, readErr := readResponse(conn)
	require.NoError(t, readErr)
	return s, p
}

// makeRawTask builds the raw OPMS task JSON (same bytes par-control forwards).
func makeRawTask(t *testing.T, bundleID, actionName string, inputs map[string]interface{}) []byte {
	t.Helper()
	inputStruct, err := structpb.NewStruct(inputs)
	require.NoError(t, err)
	pbTask := &privateactionspb.PrivateActionTask{
		TaskId: "test-001", BundleId: bundleID, ActionName: actionName,
		OrgId: 12345, Inputs: inputStruct,
	}
	pbBytes, err := proto.Marshal(pbTask)
	require.NoError(t, err)

	var task types.Task
	task.Data.ID = "test-001"
	task.Data.Type = "workflowTask"
	task.Data.Attributes = &types.Attributes{
		Name: actionName, BundleID: bundleID, JobId: "job-001", OrgId: 12345,
		SignedEnvelope: &privateactionspb.RemoteConfigSignatureEnvelope{
			Data: pbBytes, HashType: privateactionspb.HashType_SHA256,
		},
	}
	raw, err := json.Marshal(task)
	require.NoError(t, err)
	return raw
}

// ── Tests ─────────────────────────────────────────────────────────────────

func TestIPC_Ping(t *testing.T) {
	socketPath, stop := testServer(t, map[string]sets.Set[string]{})
	defer stop()
	assert.True(t, sendPing(t, socketPath), "ping should return pong 0x01")
}

func TestIPC_Execute_TestConnection(t *testing.T) {
	allowlist := map[string]sets.Set[string]{
		"com.datadoghq.remoteaction": sets.New[string]("testConnection"),
	}
	socketPath, stop := testServer(t, allowlist)
	defer stop()

	// Raw task bytes passed verbatim — no base64, no JSON wrapper.
	raw := makeRawTask(t, "com.datadoghq.remoteaction", "testConnection", nil)
	status, payload := sendExecute(t, socketPath, raw, 30)

	assert.Equal(t, statusOK, status)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal(payload, &output))
	assert.Equal(t, true, output["success"])
}

func TestIPC_Execute_AllowlistBlocked(t *testing.T) {
	socketPath, stop := testServer(t, map[string]sets.Set[string]{})
	defer stop()

	raw := makeRawTask(t, "com.datadoghq.remoteaction", "testConnection", nil)
	status, payload := sendExecute(t, socketPath, raw, 10)

	assert.Equal(t, statusErr, status)
	var errResp errorResponse
	require.NoError(t, json.Unmarshal(payload, &errResp))
	assert.NotEqual(t, int32(0), errResp.ErrorCode)
}

func TestIPC_Execute_ZeroLengthTask(t *testing.T) {
	// Zero task_len is a protocol error — server should return error or close gracefully.
	socketPath, stop := testServer(t, map[string]sets.Set[string]{})
	defer stop()

	conn := dialUDS(t, socketPath)
	defer conn.Close()

	conn.Write([]byte{frameExecute})               //nolint:errcheck
	binary.Write(conn, binary.LittleEndian, uint32(0)) //nolint:errcheck

	// Either the server closes the connection or sends an error — no panic.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var statusBuf [1]byte
	_, _ = io.ReadFull(conn, statusBuf[:])
}

func TestIPC_SOPeerCred_SameProcess(t *testing.T) {
	// Connections from the same process (same UID) must pass the SO_PEERCRED check.
	socketPath, stop := testServer(t, map[string]sets.Set[string]{})
	defer stop()
	assert.True(t, sendPing(t, socketPath), "same-process connection should pass SO_PEERCRED")
}

var _ = log.FromContext // prevent unused-import error
