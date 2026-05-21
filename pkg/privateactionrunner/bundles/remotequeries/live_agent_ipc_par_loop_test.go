// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_remotequeries_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	parapp "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	com_datadoghq_remotequeries "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remotequeries"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fusedLocalProofEnv                  = "RQ_FUSED_PROOF"
	remoteQueriesSeedProofQuery         = "SELECT 1 AS value"
	remoteQueriesFixtureTableProofQuery = "SELECT city, country FROM cities ORDER BY city"
	remoteQueriesProofQueryOverrideEnv  = "RQ_REMOTE_QUERY"
)

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

	taskID := fmt.Sprintf("remotequeries-fused-local-proof-%d", time.Now().UnixNano())
	proofQuery := remoteQueriesProofQueryFromEnv()
	inputs := map[string]interface{}{
		"integration": "postgres",
		"target":      remoteQueriesPostgresTargetFromEnv(t),
		"query":       proofQuery,
		"limits":      remoteQueriesProofLimits(proofQuery),
	}
	requestEvidence, err := json.Marshal(inputs)
	require.NoError(t, err)
	require.NotContains(t, string(requestEvidence), "password")
	require.NotContains(t, string(requestEvidence), "token")
	require.NotContains(t, string(requestEvidence), "secret")

	fqn := com_datadoghq_remotequeries.BundleID + "." + com_datadoghq_remotequeries.ExecuteActionName
	t.Logf("fakeintake task enqueued: task_id=%s action_fqn=%s inputs=%s", taskID, fqn, requestEvidence)
	t.Logf("real AgentSecure IPC configured: 127.0.0.1:%d RemoteQueryExecute", cmdPortInt)
	require.NoError(t, fakeintakeClient.EnqueuePARTask(taskID, fqn, inputs))

	result, err := fakeintakeClient.GetPARTaskResult(taskID, 20*time.Second)
	require.NoError(t, err)
	if !result.Success {
		t.Logf("failed PAR task result: %+v", result)
	}
	require.True(t, result.Success)
	require.Equal(t, taskID, result.TaskID)
	assert.Equal(t, "SUCCEEDED", result.Outputs["status"])
	require.Contains(t, result.Outputs, "rows")

	rows, ok := result.Outputs["rows"].([]interface{})
	require.True(t, ok)
	assertRemoteQueriesProofRows(t, proofQuery, rows)

	resultEvidence, err := json.Marshal(result.Outputs)
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
		fmt.Sprintf("real AgentSecure IPC called via NewDefaultBridgeClient: 127.0.0.1:%d RemoteQueryExecute", cmdPortInt),
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

func remoteQueriesProofLimits(query string) map[string]interface{} {
	maxRows := 1
	if query == remoteQueriesFixtureTableProofQuery {
		maxRows = 2
	}
	return map[string]interface{}{
		"maxRows":   maxRows,
		"maxBytes":  1024,
		"timeoutMs": 1000,
	}
}

func assertRemoteQueriesProofRows(t *testing.T, query string, rows []interface{}) {
	t.Helper()

	switch query {
	case remoteQueriesFixtureTableProofQuery:
		require.Len(t, rows, 2)
		firstRow, ok := rows[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "Beautiful city of lights", firstRow["city"])
		assert.Equal(t, "France", firstRow["country"])
		secondRow, ok := rows[1].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "New York", secondRow["city"])
		assert.Equal(t, "USA", secondRow["country"])
	case remoteQueriesSeedProofQuery:
		require.Len(t, rows, 1)
		firstRow, ok := rows[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, float64(1), firstRow["value"])
	default:
		require.FailNowf(t, "unsupported proof query", "%s=%q must use a bridge-allowlisted proof query", remoteQueriesProofQueryOverrideEnv, query)
	}
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
	payload := ""
	for _, line := range lines {
		payload += line + "\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
}

func getenvRequired(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	require.NotEmptyf(t, value, "%s is required", name)
	return value
}
