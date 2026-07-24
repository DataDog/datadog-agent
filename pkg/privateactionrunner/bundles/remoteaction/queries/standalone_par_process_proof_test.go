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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	com_datadoghq_remoteaction_queries "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/queries"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const standaloneLocalProofEnv = "RQ_STANDALONE_PROOF"

func TestRemoteQueriesActionRunsThroughStandalonePARProcessWithRealAgentIPC(t *testing.T) {
	if os.Getenv(standaloneLocalProofEnv) != "1" {
		t.Skipf("set %s=1 and start a local Agent with a loaded Postgres check to run the standalone PAR process proof", standaloneLocalProofEnv)
	}

	parBin := getenvRequired(t, "RQ_STANDALONE_PAR_BIN")
	cmdPort := getenvRequired(t, "RQ_STANDALONE_AGENT_CMD_PORT")
	authTokenFile := getenvRequired(t, "RQ_STANDALONE_AGENT_AUTH_TOKEN_FILE")
	ipcCertFile := getenvRequired(t, "RQ_STANDALONE_AGENT_IPC_CERT_FILE")
	agentPID := getenvOptional("RQ_STANDALONE_AGENT_PID")
	cmdPortInt, err := strconv.Atoi(cmdPort)
	require.NoError(t, err)

	fakeintake, _ := fakeintakeserver.InitialiseForTests(t)
	defer func() { require.NoError(t, fakeintake.Stop()) }()
	fakeintakeClient := fakeintakeclient.NewClient(fakeintake.URL())
	require.NoError(t, fakeintakeClient.FlushPAR())

	parDir := t.TempDir()
	parLog := filepath.Join(parDir, "private-action-runner.log")
	writeStandalonePARConfig(t, parDir, parLog, fakeintake.URL(), cmdPortInt, authTokenFile, ipcCertFile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, parBin, "run", "-c", parDir)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), constants.InternalSkipTaskVerificationEnvVar+"=true")
	require.NoError(t, cmd.Start())
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()
	require.NotNil(t, cmd.Process)
	parPID := cmd.Process.Pid
	t.Logf("standalone private-action-runner process started: pid=%d bin=%s cfg=%s", parPID, parBin, parDir)
	if agentPID != "" {
		parsedAgentPID, err := strconv.Atoi(agentPID)
		require.NoError(t, err)
		require.NotEqual(t, parsedAgentPID, parPID, "PAR must be a separate OS process from the Agent")
	}

	waitForStandalonePARPolling(t, fakeintakeClient, cmd, parLog, &stdout, &stderr)

	taskID := fmt.Sprintf("remoteaction-queries-standalone-par-proof-%d", time.Now().UnixNano())
	proofQuery := remoteQueriesProofQueryFromEnv()
	format := os.Getenv("RQ_REMOTE_FORMAT")
	if format == "" {
		format = "csv"
	}
	inputs := map[string]interface{}{
		"integration": "postgres",
		"operation":   "copy_stream",
		"format":      format,
		"target":      remoteQueriesPostgresTargetFromEnv(t),
		"query":       proofQuery,
		"copyLimits":  remoteQueriesProofCopyLimits(proofQuery),
	}
	requestEvidence, err := json.Marshal(inputs)
	require.NoError(t, err)
	requireNoCredentialShape(t, requestEvidence)

	fqn := com_datadoghq_remoteaction_queries.BundleID + "." + com_datadoghq_remoteaction_queries.ExecuteActionName
	t.Logf("fakeintake task enqueued: task_id=%s action_fqn=%s inputs=%s", taskID, fqn, requestEvidence)
	t.Logf("real AgentSecure IPC configured for standalone PAR: 127.0.0.1:%d RemoteQueryExecuteStream", cmdPortInt)
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
		t.Logf("PAR log tail:\n%s", readTail(parLog, 120))
	}
	require.True(t, result.Success)
	require.Equal(t, taskID, result.TaskID)
	assert.Equal(t, "SUCCEEDED", result.Outputs["status"])
	t.Logf("copy stream PAR outputs: %+v", summarizeRemoteQueriesProofPayload(result.Outputs))
	if os.Getenv("RQ_REMOTE_FORMAT") == "binary" {
		dataBytes, ok := result.Outputs["data_bytes"].(string)
		require.True(t, ok)
		assertRemoteQueriesProofBinaryCopyData(t, proofQuery, dataBytes)
	} else {
		data, ok := result.Outputs["data"].(string)
		require.True(t, ok)
		assertRemoteQueriesProofCopyData(t, proofQuery, data)
	}

	resultEvidence, err := json.Marshal(summarizeRemoteQueriesProofPayload(result.Outputs))
	require.NoError(t, err)
	requireNoCredentialShape(t, resultEvidence)
	t.Logf("fakeintake captured successful PAR task result: %s", resultEvidence)

	dequeueCalls, err := fakeintakeClient.GetPARDequeueCount()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dequeueCalls, 1)
	t.Logf("standalone PAR process dequeued from fakeintake: dequeue_calls=%d", dequeueCalls)

	writeFusedEvidence(t, getenvOptional("RQ_STANDALONE_EVIDENCE_FILE"), []string{
		fmt.Sprintf("standalone private-action-runner process pid=%d", parPID),
		"separate Agent process pid=" + agentPID,
		fmt.Sprintf("fakeintake task enqueued: task_id=%s action_fqn=%s inputs=%s", taskID, fqn, requestEvidence),
		"standalone PAR process dequeued the fakeintake OPMS task and invoked the registered action",
		fmt.Sprintf("real AgentSecure IPC called by standalone PAR: 127.0.0.1:%d RemoteQueryExecuteStream", cmdPortInt),
		fmt.Sprintf("fakeintake captured successful PAR task result: %s", resultEvidence),
		fmt.Sprintf("dequeue_calls=%d", dequeueCalls),
		"task verification skipped for this standalone tracer bullet with DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true",
	})
}

func writeStandalonePARConfig(t *testing.T, dir, logFile, fakeintakeURL string, cmdPort int, authTokenFile, ipcCertFile string) {
	t.Helper()
	privateJWK, _, err := util.GenerateKeys()
	require.NoError(t, err)
	privateJWKJSON, err := json.Marshal(privateJWK)
	require.NoError(t, err)
	privateKeyB64 := base64.RawURLEncoding.EncodeToString(privateJWKJSON)

	cfg := fmt.Sprintf(`api_key: '00000000000000000000000000000000'
dd_url: %q
hostname: rq-standalone-par-proof
cmd_host: 127.0.0.1
cmd_port: %d
auth_token_file_path: %q
ipc_cert_file_path: %q
log_level: debug
telemetry.enabled: false
inventories_enabled: false
process_config.enabled: 'false'
logs_enabled: false
apm_config.enabled: false
private_action_runner:
  enabled: true
  self_enroll: false
  urn: "urn:dd:apps:on-prem-runner:us1:123456:remoteaction-queries-standalone-par-local-proof-runner"
  private_key: %q
  log_file: %q
  default_actions_enabled: false
  actions_allowlist:
    - "com.datadoghq.remoteaction.queries.execute"
  task_concurrency: 1
  task_timeout_seconds: 120
`, fakeintakeURL, cmdPort, authTokenFile, ipcCertFile, privateKeyB64, logFile)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(cfg), 0o600))
}

func waitForStandalonePARPolling(t *testing.T, client *fakeintakeclient.Client, cmd *exec.Cmd, parLog string, stdout, stderr *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			require.FailNowf(t, "standalone PAR process exited before polling fakeintake", "stdout:\n%s\nstderr:\n%s\nlog tail:\n%s", stdout.String(), stderr.String(), readTail(parLog, 120))
		}
		if count, err := client.GetPARDequeueCount(); err == nil && count > 0 {
			t.Logf("standalone PAR process is polling fakeintake: dequeue_calls=%d", count)
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	require.FailNowf(t, "timed out waiting for standalone PAR process to poll fakeintake", "stdout:\n%s\nstderr:\n%s\nlog tail:\n%s", stdout.String(), stderr.String(), readTail(parLog, 120))
}

func requireNoCredentialShape(t *testing.T, payload []byte) {
	t.Helper()
	lower := strings.ToLower(string(payload))
	require.NotContains(t, lower, "password")
	require.NotContains(t, lower, "token")
	require.NotContains(t, lower, "secret")
}

func readTail(path string, maxLines int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("<unable to read %s: %v>", path, err)
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) <= maxLines {
		return string(content)
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}
