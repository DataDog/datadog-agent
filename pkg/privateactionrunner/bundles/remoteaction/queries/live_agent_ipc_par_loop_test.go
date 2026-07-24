// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build remoteaction_queries_live && !windows

package com_datadoghq_remoteaction_queries_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	parapp "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	com_datadoghq_remoteaction_queries "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/queries"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fusedLocalProofEnv                   = "RQ_FUSED_PROOF"
	remoteQueriesSeedProofQuery          = "SELECT 1 AS value"
	remoteQueriesFixtureTableProofQuery  = "SELECT city, country FROM cities ORDER BY city"
	remoteQueriesBinaryPayloadProofQuery = "SELECT decode('00ff80', 'hex') AS payload"
	remoteQueriesProofQueryOverrideEnv   = "RQ_REMOTE_QUERY"
)

var remoteQueriesLargePayloadProofQueries = map[string]int{
	"SELECT repeat('x', 1048576) AS payload":  1 << 20,  // 1 MiB.
	"SELECT repeat('x', 2097152) AS payload":  2 << 20,  // 2 MiB.
	"SELECT repeat('x', 4194304) AS payload":  4 << 20,  // 4 MiB.
	"SELECT repeat('x', 8388608) AS payload":  8 << 20,  // 8 MiB.
	"SELECT repeat('x', 16777216) AS payload": 16 << 20, // 16 MiB.
	"SELECT repeat('x', 33554432) AS payload": 32 << 20, // 32 MiB.
}

func TestRemoteQueriesActionRunsThroughLivePARLoopWithRealAgentIPC(t *testing.T) {
	if os.Getenv(fusedLocalProofEnv) != "1" {
		t.Skipf("set %s=1 and start a local Agent with a loaded Postgres check to run the fused local proof", fusedLocalProofEnv)
	}

	cmdPort := getenvRequired(t, "RQ_FUSED_AGENT_CMD_PORT")
	authTokenFile := getenvRequired(t, "RQ_FUSED_AGENT_AUTH_TOKEN_FILE")
	ipcCertFile := getenvRequired(t, "RQ_FUSED_AGENT_IPC_CERT_FILE")
	cmdPortInt, err := strconv.Atoi(cmdPort)
	require.NoError(t, err)

	// NewDefaultBridgeClient reads the process-wide Datadog config. Point it at the
	// separate local Agent process started by the fused proof harness so the PAR
	// action uses the real Agent IPC HTTP endpoint, not an httptest bridge.
	cfg := pkgconfigsetup.Datadog()
	cfg.SetWithoutSource("cmd_host", "127.0.0.1")
	cfg.SetWithoutSource("cmd_port", cmdPortInt)
	cfg.SetWithoutSource("auth_token_file_path", authTokenFile)
	cfg.SetWithoutSource("ipc_cert_file_path", ipcCertFile)

	t.Setenv(parapp.InternalSkipTaskVerificationEnvVar, "true")

	fakeintake, _ := fakeintakeserver.InitialiseForTests(t)
	defer func() { require.NoError(t, fakeintake.Stop()) }()
	fakeintakeClient := fakeintakeclient.NewClient(fakeintake.URL())
	require.NoError(t, fakeintakeClient.FlushPAR())

	cfgPAR := newLivePARTestConfig(t, fakeintake.URL())
	keysManager := taskverifier.NewKeyManager(nil)
	verifier := taskverifier.NewTaskVerifier(keysManager, cfgPAR)
	workflowRunner, err := runners.NewWorkflowRunner(cfgPAR, keysManager, verifier, opms.NewClient(cfgPAR), nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, workflowRunner.Start(ctx))
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		require.NoError(t, workflowRunner.Stop(stopCtx))
	}()

	taskID := fmt.Sprintf("remoteaction-queries-fused-local-proof-%d", time.Now().UnixNano())
	proofQuery := remoteQueriesProofQueryFromEnv()
	inputs := map[string]interface{}{
		"integration": "postgres",
		"operation":   "copy_stream",
		"format":      "csv",
		"target":      remoteQueriesPostgresTargetFromEnv(t),
		"query":       proofQuery,
		"copyLimits":  remoteQueriesProofCopyLimits(proofQuery),
	}
	requestEvidence, err := json.Marshal(inputs)
	require.NoError(t, err)
	require.NotContains(t, string(requestEvidence), "password")
	require.NotContains(t, string(requestEvidence), "token")
	require.NotContains(t, string(requestEvidence), "secret")

	fqn := com_datadoghq_remoteaction_queries.BundleID + "." + com_datadoghq_remoteaction_queries.ExecuteActionName
	t.Logf("fakeintake task enqueued: task_id=%s action_fqn=%s inputs=%s", taskID, fqn, requestEvidence)
	t.Logf("real AgentSecure IPC configured: 127.0.0.1:%d RemoteQueryExecuteStream", cmdPortInt)
	require.NoError(t, fakeintakeClient.EnqueuePARTask(taskID, fqn, inputs))

	result, err := fakeintakeClient.GetPARTaskResult(taskID, remoteQueriesProofResultTimeout(proofQuery))
	require.NoError(t, err)
	if !result.Success {
		t.Logf("failed PAR task result: %+v", summarizeRemoteQueriesProofPayload(map[string]interface{}{
			"task_id":       result.TaskID,
			"success":       result.Success,
			"outputs":       result.Outputs,
			"error_code":    result.ErrorCode,
			"error_details": result.ErrorDetails,
		}))
	}
	require.True(t, result.Success)
	require.Equal(t, taskID, result.TaskID)
	assert.Equal(t, "SUCCEEDED", result.Outputs["status"])
	require.Contains(t, result.Outputs, "data")

	data, ok := result.Outputs["data"].(string)
	require.True(t, ok)
	assertRemoteQueriesProofCopyData(t, proofQuery, data)

	resultEvidence, err := json.Marshal(summarizeRemoteQueriesProofPayload(result.Outputs))
	require.NoError(t, err)
	require.NotContains(t, string(resultEvidence), "password")
	require.NotContains(t, string(resultEvidence), "token")
	require.NotContains(t, string(resultEvidence), "secret")
	t.Logf("fakeintake captured successful PAR task result: %s", resultEvidence)

	dequeueCalls, err := fakeintakeClient.GetPARDequeueCount()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dequeueCalls, 1)
	t.Logf("live PAR loop dequeued from fakeintake: dequeue_calls=%d", dequeueCalls)
	writeFusedEvidence(t, getenvOptional("RQ_FUSED_EVIDENCE_FILE"), []string{
		fmt.Sprintf("fakeintake task enqueued: task_id=%s action_fqn=%s inputs=%s", taskID, fqn, requestEvidence),
		"live PAR loop dequeued the fakeintake OPMS task and invoked the registered action",
		fmt.Sprintf("real AgentSecure IPC called via NewDefaultBridgeClient: 127.0.0.1:%d RemoteQueryExecuteStream", cmdPortInt),
		fmt.Sprintf("fakeintake captured successful PAR task result: %s", resultEvidence),
		fmt.Sprintf("dequeue_calls=%d", dequeueCalls),
		"task verification skipped locally with DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true",
	})
}

func getenvOptional(name string) string {
	return os.Getenv(name)
}

func remoteQueriesProofQueryFromEnv() string {
	if query := os.Getenv(remoteQueriesProofQueryOverrideEnv); query != "" {
		return query
	}
	return remoteQueriesFixtureTableProofQuery
}

func remoteQueriesProofCopyLimits(query string) map[string]interface{} {
	maxBytes := 4 << 10 // 4 KiB.
	timeoutMs := 5_000
	if payloadBytes, ok := remoteQueriesLargePayloadBytes(query); ok {
		maxBytes = payloadBytes + (1 << 20) // Add 1 MiB headroom.
		timeoutMs = 60_000
	}
	chunkBytes := 256 << 10 // 256 KiB.
	if value := os.Getenv("RQ_REMOTE_CHUNK_BYTES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			chunkBytes = parsed
		}
	}
	return map[string]interface{}{
		"chunkBytes":  chunkBytes,
		"maxBytes":    maxBytes,
		"maxRowBytes": maxBytes,
		"timeoutMs":   timeoutMs,
	}
}

func remoteQueriesProofResultTimeout(query string) time.Duration {
	if _, ok := remoteQueriesLargePayloadBytes(query); ok {
		return 2 * time.Minute
	}
	return 30 * time.Second
}

func remoteQueriesLargePayloadBytes(query string) (int, bool) {
	payloadBytes, ok := remoteQueriesLargePayloadProofQueries[query]
	return payloadBytes, ok
}

func assertRemoteQueriesProofBinaryCopyData(t *testing.T, query string, dataBase64 string) {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString(dataBase64)
	require.NoError(t, err)
	switch query {
	case remoteQueriesBinaryPayloadProofQuery:
		assert.True(t, bytes.HasPrefix(data, []byte("PGCOPY\n\xff\r\n\x00")), "binary COPY payload should keep the PostgreSQL binary header")
		assert.Contains(t, data, byte(0x00))
		assert.True(t, bytes.Contains(data, []byte{0x00, 0xff, 0x80}), "binary COPY payload should contain the row bytes from decode('00ff80','hex')")
	default:
		require.FailNowf(t, "unsupported binary COPY proof query", "%s=%q must use a binary COPY bridge-allowlisted proof query", remoteQueriesProofQueryOverrideEnv, query)
	}
}

func assertRemoteQueriesProofCopyData(t *testing.T, query string, data string) {
	t.Helper()

	switch query {
	case remoteQueriesFixtureTableProofQuery:
		assert.Equal(t, "Beautiful city of lights,France\nNew York,USA\n", data)
	case remoteQueriesSeedProofQuery:
		assert.Equal(t, "1\n", data)
	default:
		expectedPayloadBytes, ok := remoteQueriesLargePayloadBytes(query)
		if ok {
			assert.Len(t, data, expectedPayloadBytes+1)
			assert.Equal(t, "\n", data[len(data)-1:])
			return
		}
		require.FailNowf(t, "unsupported COPY proof query", "%s=%q must use a COPY bridge-allowlisted proof query", remoteQueriesProofQueryOverrideEnv, query)
	}
}

func summarizeRemoteQueriesProofPayload(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		copy := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			copy[key] = summarizeRemoteQueriesProofPayload(nested)
		}
		return copy
	case []interface{}:
		copy := make([]interface{}, 0, len(typed))
		for _, nested := range typed {
			copy = append(copy, summarizeRemoteQueriesProofPayload(nested))
		}
		return copy
	case string:
		if len(typed) > 4096 {
			return fmt.Sprintf("<%d bytes>", len(typed))
		}
	}
	return value
}

func remoteQueriesPostgresTargetFromEnv(t *testing.T) map[string]interface{} {
	t.Helper()

	port := 5432
	if value := os.Getenv("RQ_POSTGRES_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		require.NoError(t, err)
		port = parsed
	}

	host := os.Getenv("RQ_POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	dbname := os.Getenv("RQ_POSTGRES_DBNAME")
	if dbname == "" {
		dbname = "datadog_test"
	}

	return map[string]interface{}{
		"host":   host,
		"port":   port,
		"dbname": dbname,
	}
}

func writeFusedEvidence(t *testing.T, path string, lines []string) {
	t.Helper()
	if path == "" {
		return
	}
	var payload strings.Builder
	for _, line := range lines {
		payload.WriteString(line)
		payload.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(path, []byte(payload.String()), 0o600))
}

func getenvRequired(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	require.NotEmptyf(t, value, "%s is required", name)
	return value
}
