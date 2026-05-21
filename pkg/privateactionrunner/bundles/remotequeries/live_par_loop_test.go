// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_remotequeries_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	com_datadoghq_remotequeries "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remotequeries"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestRemoteQueriesActionRunsThroughLivePARLoopAndFakeintake(t *testing.T) {
	t.Setenv(app.InternalSkipTaskVerificationEnvVar, "true")

	bridgeRequests := make(chan []byte, 1)
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, com_datadoghq_remotequeries.AgentRemoteQueryExecuteEndpointPath, r.URL.Path)
		require.Contains(t, r.Header.Get("Content-Type"), "application/json")

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		bridgeRequests <- body

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"SUCCEEDED","rows":[{"value":1}]}`))
	}))
	defer bridgeServer.Close()

	restoreFactory := com_datadoghq_remotequeries.SetBridgeClientFactoryForTest(func() (com_datadoghq_remotequeries.BridgeClient, string, error) {
		return httpBridgeClient{}, bridgeServer.URL + com_datadoghq_remotequeries.AgentRemoteQueryExecuteEndpointPath, nil
	})
	defer restoreFactory()

	fakeintake, _ := fakeintakeserver.InitialiseForTests(t)
	defer func() { require.NoError(t, fakeintake.Stop()) }()
	fakeintakeClient := fakeintakeclient.NewClient(fakeintake.URL())
	require.NoError(t, fakeintakeClient.FlushPAR())

	cfg := newLivePARTestConfig(t, fakeintake.URL())
	keysManager := taskverifier.NewKeyManager(nil)
	verifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	workflowRunner, err := runners.NewWorkflowRunner(cfg, keysManager, verifier, opms.NewClient(cfg), nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, workflowRunner.Start(ctx))
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		require.NoError(t, workflowRunner.Stop(stopCtx))
	}()

	taskID := "remotequeries-live-par-local-proof"
	require.NoError(t, fakeintakeClient.EnqueuePARTask(taskID, com_datadoghq_remotequeries.BundleID+"."+com_datadoghq_remotequeries.ExecuteActionName, map[string]interface{}{
		"integration": "postgres",
		"target": map[string]interface{}{
			"host":   "localhost",
			"port":   5432,
			"dbname": "postgres",
		},
		"query": "SELECT 1 AS value",
		"limits": map[string]interface{}{
			"maxRows":   1,
			"maxBytes":  1024,
			"timeoutMs": 1000,
		},
	}))

	result, err := fakeintakeClient.GetPARTaskResult(taskID, 10*time.Second)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, taskID, result.TaskID)
	assert.Equal(t, "SUCCEEDED", result.Outputs["status"])
	require.Contains(t, result.Outputs, "rows")

	var bridgeRequest map[string]interface{}
	select {
	case body := <-bridgeRequests:
		require.NotContains(t, string(body), "password")
		require.NotContains(t, string(body), "token")
		require.NotContains(t, string(body), "secret")
		require.NoError(t, json.Unmarshal(body, &bridgeRequest))
	case <-time.After(2 * time.Second):
		require.FailNow(t, "remote query action did not call the local bridge")
	}
	assert.Equal(t, "postgres", bridgeRequest["integration"])
	assert.Equal(t, "SELECT 1 AS value", bridgeRequest["query"])
	assert.Equal(t, map[string]interface{}{"host": "localhost", "port": float64(5432), "dbname": "postgres"}, bridgeRequest["target"])

	dequeueCalls, err := fakeintakeClient.GetPARDequeueCount()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dequeueCalls, 1)
}

func newLivePARTestConfig(t *testing.T, fakeintakeURL string) *parconfig.Config {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	taskTimeoutSeconds := int32(5)

	return &parconfig.Config{
		ActionsAllowlist: map[string]sets.Set[string]{
			com_datadoghq_remotequeries.BundleID: sets.New[string](com_datadoghq_remotequeries.ExecuteActionName),
		},
		DDHost:             fakeintakeURL,
		DDApiHost:          "unused.local",
		DatadogSite:        "local",
		OrgId:              123456,
		PrivateKey:         privateKey,
		RunnerId:           "remotequeries-live-par-local-proof-runner",
		Urn:                "urn:dd:apps:on-prem-runner:us1:123456:remotequeries-live-par-local-proof-runner",
		LoopInterval:       10 * time.Millisecond,
		MinBackoff:         10 * time.Millisecond,
		MaxBackoff:         50 * time.Millisecond,
		WaitBeforeRetry:    50 * time.Millisecond,
		MaxAttempts:        3,
		OpmsRequestTimeout: 1000,
		RunnerPoolSize:     1,
		HeartbeatInterval:  time.Hour,
		TaskTimeoutSeconds: &taskTimeoutSeconds,
		MetricsClient:      &statsd.NoOpClient{},
		Tags:               []observability.Tag{},
	}
}

type httpBridgeClient struct{}

func (httpBridgeClient) Post(url string, contentType string, body io.Reader, _ ...ipc.RequestOption) ([]byte, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(url, contentType, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return respBody, assert.AnError
	}
	return respBody, nil
}
