// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package rcx509clientimpl

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries"
	remotequeriesv1alpha1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/agent_integrations/remote_queries"
)

const (
	testTask   = "d25e7f9f-6883-4462-b31e-b449f70c0894"
	testUpload = "5b0f2c63-62a8-4b2f-9d0c-2a6d4a0d9b11"
	testCrawl  = "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c"
)

type fakeQueryCheck struct {
	stub.StubCheck
	resolves, executes atomic.Int32
	executed           chan string
	// resolveGate, if set, holds each resolve until it is closed.
	resolveGate chan struct{}
}

func (c *fakeQueryCheck) String() string              { return "postgres" }
func (c *fakeQueryCheck) ID() checkid.ID              { return "postgres:1" }
func (c *fakeQueryCheck) SupportsRemoteQueries() bool { return true }
func (c *fakeQueryCheck) RemoteQuery(_ context.Context, input string) (string, error) {
	var request struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		return "", err
	}
	if request.Operation == "resolve" {
		c.resolves.Add(1)
		if c.resolveGate != nil {
			<-c.resolveGate
		}
		return `{"status":"MATCHED"}`, nil
	}
	c.executes.Add(1)
	c.executed <- input
	return `{"status":"SUCCEEDED"}`, nil
}

type fakeAuthority struct {
	trust remotequeries.TunnelTrust
	err   error
}

func (a fakeAuthority) Authorize(context.Context, uint64) (remotequeries.TunnelTrust, error) {
	return a.trust, a.err
}

func newExecuteHandler(t *testing.T, authority remotequeries.TunnelAuthority, remoteQueriesEnabled bool) (rcx509client.ExecuteHandler, *fakeQueryCheck) {
	t.Helper()
	c := &fakeQueryCheck{executed: make(chan string, 4)}
	conf := cfgcomp.NewMockWithOverrides(t, map[string]interface{}{"remote_queries.enabled": remoteQueriesEnabled})
	logger := logmock.New(t)
	service := &remotequeries.Service{Config: conf, Checks: func() []check.Check { return []check.Check{c} }, Log: nil}
	tunnel, err := remotequeries.NewTunnel(service, authority)
	require.NoError(t, err)
	return NewRemoteQueriesExecuteHandler(tunnel, logger), c
}

func trustedAuthority() remotequeries.TunnelAuthority {
	return fakeAuthority{trust: remotequeries.TunnelTrust{OrgID: 42, ConnectionID: "conn-1"}}
}

func executeRequest() *remotequeriesv1alpha1.ExecuteRequest {
	return &remotequeriesv1alpha1.ExecuteRequest{
		ConnectionId: "conn-1", OrgId: 42, Integration: "postgres",
		Target:        &remotequeriesv1alpha1.Target{DatabaseInstance: "orders"},
		Query:         "select 1",
		TaskId:        testTask,
		RunId:         testUpload,
		IncludeSchema: true,
		Limits: &remotequeriesv1alpha1.ResultLimits{
			MaxFileBytes: 16777216, MaxResultBytes: 107374182400, MaxRowBytes: 16777216, MaxColumns: 1024,
			MaxSchemaBytes: 1048576, MaxPages: 1280, TimeoutMs: 3000000,
		},
	}
}

func call(t *testing.T, handler rcx509client.ExecuteHandler, request *remotequeriesv1alpha1.ExecuteRequest) *remotequeriesv1alpha1.ExecuteResponse {
	t.Helper()
	payload, err := proto.Marshal(request)
	require.NoError(t, err)
	raw, err := handler(context.Background(), 7, payload)
	require.NoError(t, err)
	var response remotequeriesv1alpha1.ExecuteResponse
	require.NoError(t, proto.Unmarshal(raw, &response))
	return &response
}

func TestExecuteHandlerResolveOnly(t *testing.T) {
	handler, c := newExecuteHandler(t, trustedAuthority(), true)
	response := call(t, handler, &remotequeriesv1alpha1.ExecuteRequest{
		ConnectionId: "conn-1", OrgId: 42, Integration: "postgres", ResolveOnly: true,
		Target: &remotequeriesv1alpha1.Target{Host: "db", Port: 5432, Dbname: "orders"},
	})
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_MATCHED, response.Status)
	require.EqualValues(t, 1, c.resolves.Load())
	require.Zero(t, c.executes.Load())

	// A resolve with SQL, limits or a run identity is not a resolve.
	bad := &remotequeriesv1alpha1.ExecuteRequest{ConnectionId: "conn-1", OrgId: 42, Integration: "postgres", ResolveOnly: true, Query: "select 1",
		Target: &remotequeriesv1alpha1.Target{DatabaseInstance: "orders"}}
	response = call(t, handler, bad)
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
	require.Equal(t, "invalid_request", response.ErrorCode)
}

// The marshaled wire message reaches the check in the bridge shape: the real
// task ID, the run ID as the upload ID, and no crawl field.
func TestExecuteHandlerBridgeShape(t *testing.T) {
	handler, c := newExecuteHandler(t, trustedAuthority(), true)
	response := call(t, handler, executeRequest())
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ACCEPTED, response.Status)
	require.Empty(t, response.ErrorCode)
	select {
	case got := <-c.executed:
		require.JSONEq(t, `{"operation":"execute","target":{"database_instance":"orders"},"query":"select 1","includeSchema":true,`+
			`"resultDelivery":{"taskId":"`+testTask+`","uploadId":"`+testUpload+`","baseUrl":"https://api.datadoghq.com/api/unstable/its-agent-intake",`+
			`"limits":{"maxFileBytes":16777216,"maxResultBytes":107374182400,"maxRowBytes":16777216,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":1280,"timeoutMs":3000000}}}`, got)
		require.NotContains(t, got, testCrawl)
		require.NotContains(t, got, "crawl")
	case <-time.After(3 * time.Second):
		t.Fatal("execute did not start")
	}

	// A duplicate is acknowledged, a conflicting reuse of the run ID is rejected, neither executes again.
	response = call(t, handler, executeRequest())
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ALREADY_ACCEPTED, response.Status)
	conflicting := executeRequest()
	conflicting.Query = "select 2"
	response = call(t, handler, conflicting)
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
	require.Equal(t, "conflict", response.ErrorCode)
	require.EqualValues(t, 1, c.executes.Load())
}

// An identical request overlapping the first is confirmed after the handoff or,
// if that is not settled, left unconfirmed: never a definite refusal, since the
// worker would abort the session the first request may be starting.
func TestExecuteHandlerOverlappingDuplicate(t *testing.T) {
	handler, c := newExecuteHandler(t, trustedAuthority(), true)
	c.resolveGate = make(chan struct{})
	payload, err := proto.Marshal(executeRequest())
	require.NoError(t, err)
	type result struct {
		raw []byte
		err error
	}
	first := make(chan result, 1)
	go func() {
		raw, err := handler(context.Background(), 1, payload)
		first <- result{raw, err}
	}()
	require.Eventually(t, func() bool { return c.resolves.Load() == 1 }, 3*time.Second, time.Millisecond)

	// The duplicate's own deadline passes while the first is unsettled.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	raw, err := handler(ctx, 2, payload)
	require.NoError(t, err)
	var response remotequeriesv1alpha1.ExecuteResponse
	require.NoError(t, proto.Unmarshal(raw, &response))
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_UNSPECIFIED, response.Status)
	require.NotEqual(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
	require.Empty(t, response.ErrorCode)
	require.Zero(t, c.executes.Load())

	// A duplicate that waits is confirmed once the first handoff has happened.
	second := make(chan result, 1)
	go func() {
		raw, err := handler(context.Background(), 3, payload)
		second <- result{raw, err}
	}()
	time.Sleep(20 * time.Millisecond)
	close(c.resolveGate)
	for name, ch := range map[string]chan result{"first": first, "second": second} {
		got := <-ch
		require.NoError(t, got.err, name)
		var response remotequeriesv1alpha1.ExecuteResponse
		require.NoError(t, proto.Unmarshal(got.raw, &response), name)
		if name == "first" {
			require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ACCEPTED, response.Status)
		} else {
			require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ALREADY_ACCEPTED, response.Status)
		}
	}
	select {
	case <-c.executed:
	case <-time.After(3 * time.Second):
		t.Fatal("execute did not start")
	}
	require.EqualValues(t, 1, c.executes.Load())
	require.EqualValues(t, 1, c.resolves.Load())
}

func TestExecuteHandlerNegatives(t *testing.T) {
	for name, tc := range map[string]struct {
		authority remotequeries.TunnelAuthority
		enabled   bool
		change    func(*remotequeriesv1alpha1.ExecuteRequest)
		code      string
	}{
		"authority denies":   {fakeAuthority{err: errors.New("no")}, true, nil, "unauthorized"},
		"zero trust":         {fakeAuthority{}, true, nil, "unauthorized"},
		"org from message":   {trustedAuthority(), true, func(r *remotequeriesv1alpha1.ExecuteRequest) { r.OrgId = 7 }, "unauthorized"},
		"connection message": {trustedAuthority(), true, func(r *remotequeriesv1alpha1.ExecuteRequest) { r.ConnectionId = "x" }, "unauthorized"},
		"local opt-in off":   {trustedAuthority(), false, nil, "disabled"},
		"missing limits":     {trustedAuthority(), true, func(r *remotequeriesv1alpha1.ExecuteRequest) { r.Limits = nil }, "invalid_request"},
		"missing target":     {trustedAuthority(), true, func(r *remotequeriesv1alpha1.ExecuteRequest) { r.Target = nil }, "invalid_request"},
		"run is not uuid":    {trustedAuthority(), true, func(r *remotequeriesv1alpha1.ExecuteRequest) { r.RunId = "upload-1" }, "invalid_request"},
		"missing task":       {trustedAuthority(), true, func(r *remotequeriesv1alpha1.ExecuteRequest) { r.TaskId = "" }, "invalid_request"},
	} {
		t.Run(name, func(t *testing.T) {
			handler, c := newExecuteHandler(t, tc.authority, tc.enabled)
			request := executeRequest()
			if tc.change != nil {
				tc.change(request)
			}
			response := call(t, handler, request)
			require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
			require.Equal(t, tc.code, response.ErrorCode)
			require.NotEmpty(t, response.ErrorMessage)
			require.Zero(t, c.resolves.Load())
			require.Zero(t, c.executes.Load())
		})
	}
}

func TestExecuteHandlerMalformedPayloadIsRejectedNotAnError(t *testing.T) {
	handler, c := newExecuteHandler(t, trustedAuthority(), true)
	raw, err := handler(context.Background(), 1, []byte("not protobuf"))
	require.NoError(t, err)
	var response remotequeriesv1alpha1.ExecuteResponse
	require.NoError(t, proto.Unmarshal(raw, &response))
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
	require.Zero(t, c.resolves.Load())
}

// The component registers the Execute URI only when a handler with its own
// trusted authorization is provided. Production provides none, and enabling
// the x509 client or the debug ping does not change that.
func TestExecuteHandlerRegistrationIsOptIn(t *testing.T) {
	var got rcx509client.ClientConfig
	factory := func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		got = config
		return newFakeClient(), nil
	}
	deps, _ := newDependencies(t, map[string]interface{}{enabledSetting: true, debugPingEnabledSetting: true, "remote_queries.enabled": true}, factory)
	New(deps)
	require.Nil(t, got.ExecuteHandler)

	handler, _ := newExecuteHandler(t, trustedAuthority(), true)
	deps, _ = newDependencies(t, map[string]interface{}{enabledSetting: true}, factory)
	deps.ExecuteHandler = handler
	New(deps)
	require.NotNil(t, got.ExecuteHandler)
}

func TestExecuteURIIsTheV1Alpha1Method(t *testing.T) {
	require.Equal(t, "rc.x509.magic_tunnel.agent_integrations.remote_queries.v1alpha1.RemoteQueriesService/Execute", remoteQueriesExecuteURI)
	require.Contains(t, string((&remotequeriesv1alpha1.ExecuteRequest{}).ProtoReflect().Descriptor().ParentFile().Package()), "remote_queries.v1alpha1")
}
