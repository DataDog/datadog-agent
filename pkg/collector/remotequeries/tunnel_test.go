// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package remotequeries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	tunnelTask   = "d25e7f9f-6883-4462-b31e-b449f70c0894"
	tunnelUpload = "5b0f2c63-62a8-4b2f-9d0c-2a6d4a0d9b11"
)

type fakeAuthority struct {
	trust TunnelTrust
	err   error
	calls atomic.Int32
}

func (a *fakeAuthority) Authorize(context.Context, uint64) (TunnelTrust, error) {
	a.calls.Add(1)
	return a.trust, a.err
}

func tunnelExecuteRequest() TunnelRequest {
	return TunnelRequest{
		ConnectionID: "conn-1", OrgID: 42, Integration: "postgres",
		Target: Target{DatabaseInstance: "orders"}, Query: "select 1", TaskID: tunnelTask, RunID: tunnelUpload,
		Limits: &TunnelLimits{MaxFileBytes: 16777216, MaxResultBytes: 107374182400, MaxRowBytes: 16777216, MaxColumns: 1024, MaxSchemaBytes: 1048576, MaxPages: 1280, TimeoutMs: 3000000},
	}
}

func tunnelResolveRequest() TunnelRequest {
	return TunnelRequest{ConnectionID: "conn-1", OrgID: 42, Integration: "postgres", ResolveOnly: true, Target: Target{Host: "db", Port: 5432, DBName: "orders"}}
}

type tunnelFixture struct {
	tunnel *Tunnel
	auth   *fakeAuthority
	conf   config.Component
	logs   *recordingLog
}

func newTunnelFixture(t *testing.T, checks ...check.Check) *tunnelFixture {
	t.Helper()
	conf := config.NewMock(t)
	// Only the local Remote Queries opt-in is set: the PAR allowlist is not a
	// substitute for tunnel authorization and is deliberately left empty.
	conf.SetInTest("remote_queries.enabled", true)
	logs := &recordingLog{}
	auth := &fakeAuthority{trust: TunnelTrust{OrgID: 42, ConnectionID: "conn-1"}}
	tunnel, err := NewTunnel(&Service{Config: conf, Checks: func() []check.Check { return checks }, Log: logs}, auth)
	require.NoError(t, err)
	return &tunnelFixture{tunnel, auth, conf, logs}
}

func TestTunnelRequiresAuthority(t *testing.T) {
	conf := config.NewMock(t)
	_, err := NewTunnel(&Service{Config: conf}, nil)
	require.Error(t, err)
	_, err = NewTunnel(nil, &fakeAuthority{})
	require.Error(t, err)
}

func TestTunnelResolveOnly(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newTunnelFixture(t, c)
	outcome := f.tunnel.Execute(context.Background(), 1, tunnelResolveRequest())
	require.Equal(t, TunnelMatched, outcome.Status)
	require.EqualValues(t, 1, c.resolves.Load())
	require.Zero(t, c.executes.Load())

	// Missing and ambiguous targets are rejected with their own codes.
	none := newTunnelFixture(t)
	outcome = none.tunnel.Execute(context.Background(), 1, tunnelResolveRequest())
	require.Equal(t, TunnelRejected, outcome.Status)
	require.Equal(t, "target_not_found", outcome.Code)
	two := newTunnelFixture(t, newQueryCheck("a"), newQueryCheck("b"))
	outcome = two.tunnel.Execute(context.Background(), 1, tunnelResolveRequest())
	require.Equal(t, "ambiguous_target", outcome.Code)
}

func TestTunnelResolveOnlyShape(t *testing.T) {
	for name, change := range map[string]func(*TunnelRequest){
		"query":        func(r *TunnelRequest) { r.Query = "select 1" },
		"schema":       func(r *TunnelRequest) { r.IncludeSchema = true },
		"task":         func(r *TunnelRequest) { r.TaskID = tunnelTask },
		"run":          func(r *TunnelRequest) { r.RunID = tunnelUpload },
		"limits":       func(r *TunnelRequest) { r.Limits = &TunnelLimits{} },
		"no target":    func(r *TunnelRequest) { r.Target = Target{} },
		"mixed target": func(r *TunnelRequest) { r.Target.DatabaseInstance = "orders" },
		"bad port":     func(r *TunnelRequest) { r.Target.Port = 70000 },
		"bad name":     func(r *TunnelRequest) { r.Integration = "post gres" },
	} {
		t.Run(name, func(t *testing.T) {
			c := newQueryCheck("candidate")
			f := newTunnelFixture(t, c)
			request := tunnelResolveRequest()
			change(&request)
			outcome := f.tunnel.Execute(context.Background(), 1, request)
			require.Equal(t, TunnelRejected, outcome.Status)
			require.Equal(t, "invalid_request", outcome.Code)
			require.Zero(t, c.resolves.Load())
		})
	}
}

// The check receives the bridge shape: the real task ID, the run ID as the
// upload ID, the locally chosen destination, the limits, and no crawl field.
func TestTunnelExecuteBridgeShape(t *testing.T) {
	c := newQueryCheck("candidate")
	received := make(chan string, 1)
	c.execute = func(_ context.Context, input string) (string, error) {
		received <- input
		return `{"status":"SUCCEEDED"}`, nil
	}
	f := newTunnelFixture(t, c)
	outcome := f.tunnel.Execute(context.Background(), 1, tunnelExecuteRequest())
	require.Equal(t, TunnelAccepted, outcome.Status)
	select {
	case got := <-received:
		require.JSONEq(t, `{"operation":"execute","target":{"database_instance":"orders"},"query":"select 1","includeSchema":false,`+
			`"resultDelivery":{"taskId":"`+tunnelTask+`","uploadId":"`+tunnelUpload+`","baseUrl":"https://api.datadoghq.com/api/unstable/its-agent-intake",`+
			`"limits":{"maxFileBytes":16777216,"maxResultBytes":107374182400,"maxRowBytes":16777216,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":1280,"timeoutMs":3000000}}}`, got)
		require.NotContains(t, got, "crawl")
		require.NotContains(t, got, "traceContext")
	case <-time.After(3 * time.Second):
		t.Fatal("execute did not start")
	}
	line := f.logs.waitRunLog(t)
	require.Contains(t, line, `upload_id="`+tunnelUpload+`"`)
	require.NotContains(t, line, "crawl")
}

func TestTunnelExecuteRoutesUsingNormalizedDatabaseHost(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newTunnelFixture(t, c)
	request := tunnelExecuteRequest()
	request.Integration = " Postgres "
	request.Target = Target{Host: "DB.", Port: 5432, DBName: "orders"}
	require.Equal(t, TunnelAccepted, f.tunnel.Execute(context.Background(), 1, request).Status)
	f.logs.waitRunLog(t)
}

func TestTunnelUnauthorizedNeverReachesChecks(t *testing.T) {
	for name, setup := range map[string]func(*tunnelFixture, *TunnelRequest){
		"authority denies":       func(f *tunnelFixture, _ *TunnelRequest) { f.auth.err = errors.New("no") },
		"zero trust":             func(f *tunnelFixture, _ *TunnelRequest) { f.auth.trust = TunnelTrust{} },
		"no connection in trust": func(f *tunnelFixture, _ *TunnelRequest) { f.auth.trust.ConnectionID = "" },
		"no org in trust":        func(f *tunnelFixture, _ *TunnelRequest) { f.auth.trust.OrgID = 0 },
		"message org differs":    func(_ *tunnelFixture, r *TunnelRequest) { r.OrgID = 43 },
		"message org missing":    func(_ *tunnelFixture, r *TunnelRequest) { r.OrgID = 0 },
		"connection differs":     func(_ *tunnelFixture, r *TunnelRequest) { r.ConnectionID = "conn-2" },
		"connection missing":     func(_ *tunnelFixture, r *TunnelRequest) { r.ConnectionID = "" },
	} {
		for _, resolve := range []bool{true, false} {
			t.Run(name, func(t *testing.T) {
				c := newQueryCheck("candidate")
				f := newTunnelFixture(t, c)
				request := tunnelExecuteRequest()
				if resolve {
					request = tunnelResolveRequest()
				}
				setup(f, &request)
				outcome := f.tunnel.Execute(context.Background(), 1, request)
				require.Equal(t, TunnelRejected, outcome.Status)
				require.Equal(t, "unauthorized", outcome.Code)
				require.Zero(t, c.resolves.Load())
				require.Zero(t, c.executes.Load())
			})
		}
	}
}

func TestTunnelRequiresLocalOptIn(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newTunnelFixture(t, c)
	f.conf.SetInTest("remote_queries.enabled", false)
	// The PAR allowlist does not stand in for the opt-in, nor the reverse.
	f.conf.SetInTest("private_action_runner.actions_allowlist", []string{"com.datadoghq.remoteaction.integrations.runQuery"})
	for _, request := range []TunnelRequest{tunnelResolveRequest(), tunnelExecuteRequest()} {
		outcome := f.tunnel.Execute(context.Background(), 1, request)
		require.Equal(t, "disabled", outcome.Code)
	}
	require.Zero(t, c.resolves.Load())
	require.Zero(t, c.executes.Load())
}

func TestTunnelExecuteRejectsInvalidRequests(t *testing.T) {
	for name, change := range map[string]func(*TunnelRequest){
		"no query":         func(r *TunnelRequest) { r.Query = "  " },
		"no task":          func(r *TunnelRequest) { r.TaskID = "" },
		"task not uuid":    func(r *TunnelRequest) { r.TaskID = "task-1" },
		"no run":           func(r *TunnelRequest) { r.RunID = "" },
		"run not uuid":     func(r *TunnelRequest) { r.RunID = "upload-1" },
		"run path":         func(r *TunnelRequest) { r.RunID = "../" + tunnelUpload },
		"no limits":        func(r *TunnelRequest) { r.Limits = nil },
		"zero limit":       func(r *TunnelRequest) { r.Limits.MaxPages = 0 },
		"negative limit":   func(r *TunnelRequest) { r.Limits.TimeoutMs = -1 },
		"no target":        func(r *TunnelRequest) { r.Target = Target{} },
		"no integration":   func(r *TunnelRequest) { r.Integration = "" },
		"mixed target":     func(r *TunnelRequest) { r.Target.Host = "db" },
		"partial hostport": func(r *TunnelRequest) { r.Target = Target{Host: "db"} },
	} {
		t.Run(name, func(t *testing.T) {
			c := newQueryCheck("candidate")
			f := newTunnelFixture(t, c)
			request := tunnelExecuteRequest()
			limits := *request.Limits
			request.Limits = &limits
			change(&request)
			outcome := f.tunnel.Execute(context.Background(), 1, request)
			require.Equal(t, TunnelRejected, outcome.Status)
			require.Equal(t, "invalid_request", outcome.Code)
			require.Zero(t, c.resolves.Load())
			require.Zero(t, c.executes.Load())
		})
	}
}

func TestTunnelExecuteFailsClosedForUnknownSite(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newTunnelFixture(t, c)
	f.conf.SetInTest("site", "example.test")
	f.conf.SetInTest("dd_url", "https://attacker.example")
	outcome := f.tunnel.Execute(context.Background(), 1, tunnelExecuteRequest())
	require.Equal(t, "integration_unavailable", outcome.Code)
	require.Zero(t, c.executes.Load())
}

// A failed handoff releases its in-flight slot.
func TestTunnelFailedHandoffReleasesSlot(t *testing.T) {
	var mu sync.Mutex
	var checks []check.Check
	f := newTunnelFixture(t)
	f.tunnel.service.Checks = func() []check.Check {
		mu.Lock()
		defer mu.Unlock()
		return checks
	}
	outcome := f.tunnel.Execute(context.Background(), 1, tunnelExecuteRequest())
	require.Equal(t, "target_not_found", outcome.Code)
	require.Equal(t, 0, inFlight(f.tunnel))

	c := newQueryCheck("candidate")
	mu.Lock()
	checks = []check.Check{c}
	mu.Unlock()
	require.Equal(t, TunnelAccepted, f.tunnel.Execute(context.Background(), 2, tunnelExecuteRequest()).Status)
	f.logs.waitRunLog(t)
	require.EqualValues(t, 1, c.executes.Load())
}

// A run can end before start returns to the caller. The slot is released
// exactly once, however the two orderings fall.
func TestTunnelRunEndingBeforeStartReturns(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newTunnelFixture(t, c)
	for i := 0; i < 50; i++ {
		request := tunnelExecuteRequest()
		request.RunID = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		// Earlier runs may still be ending and hold the in-flight slots.
		require.Eventually(t, func() bool {
			return f.tunnel.Execute(context.Background(), uint64(i), request).Status == TunnelAccepted
		}, 3*time.Second, time.Millisecond)
	}
	require.Eventually(t, func() bool { return inFlight(f.tunnel) == 0 }, 3*time.Second, time.Millisecond)
}

// The tunnel keeps no per-run record, so the number of runs it has served does
// not limit the runs it accepts: every one of them is admitted once its
// predecessor has ended.
func TestTunnelAcceptsUnboundedSequentialRuns(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newTunnelFixture(t, c)
	for i := 0; i < 1100; i++ {
		request := tunnelExecuteRequest()
		request.RunID = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		outcome := f.tunnel.Execute(context.Background(), uint64(i), request)
		require.Equal(t, TunnelAccepted, outcome.Status, "run %d: %+v", i, outcome)
		require.Eventually(t, func() bool { return inFlight(f.tunnel) == 0 }, 3*time.Second, 100*time.Microsecond)
	}
	require.EqualValues(t, 1100, c.executes.Load())
}

func inFlight(t *Tunnel) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inFlight
}

func TestTunnelBoundsConcurrentRuns(t *testing.T) {
	c := newQueryCheck("candidate")
	release := make(chan struct{})
	c.execute = func(context.Context, string) (string, error) {
		<-release
		return `{"status":"SUCCEEDED"}`, nil
	}
	f := newTunnelFixture(t, c)
	uploads := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444", "55555555-5555-4555-8555-555555555555"}
	for i, upload := range uploads[:tunnelMaxInFlight] {
		request := tunnelExecuteRequest()
		request.RunID = upload
		require.Equal(t, TunnelAccepted, f.tunnel.Execute(context.Background(), uint64(i), request).Status)
	}
	request := tunnelExecuteRequest()
	request.RunID = uploads[tunnelMaxInFlight]
	outcome := f.tunnel.Execute(context.Background(), 9, request)
	require.Equal(t, "busy", outcome.Code)
	// The refusal did not take a slot: once the runs end, the same request is admitted.
	require.Equal(t, tunnelMaxInFlight, inFlight(f.tunnel))

	close(release)
	require.Eventually(t, func() bool {
		return f.tunnel.Execute(context.Background(), 11, request).Status == TunnelAccepted
	}, 3*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return inFlight(f.tunnel) == 0 }, 3*time.Second, time.Millisecond)
}

func TestTunnelExecuteFailureIsDetachedAndDoesNotLeak(t *testing.T) {
	c := newQueryCheck("candidate")
	c.execute = func(context.Context, string) (string, error) {
		return `{"status":"FAILED","error":{"code":"database_error","message":"secret_value row 7"}}`, nil
	}
	f := newTunnelFixture(t, c)
	outcome := f.tunnel.Execute(context.Background(), 1, tunnelExecuteRequest())
	require.Equal(t, TunnelAccepted, outcome.Status)
	line := f.logs.waitRunLog(t)
	require.Contains(t, line, "outcome=FAILED")
	require.Eventually(t, func() bool { return inFlight(f.tunnel) == 0 }, 3*time.Second, time.Millisecond)
}

func TestTunnelRejectionsCarryFixedMessages(t *testing.T) {
	c := newQueryCheck("candidate")
	c.resolve = func(context.Context, string) (string, error) {
		return `{"status":"FAILED","error":{"code":"database_error","message":"password=hunter2"}}`, nil
	}
	f := newTunnelFixture(t, c)
	outcome := f.tunnel.Execute(context.Background(), 1, tunnelResolveRequest())
	require.Equal(t, "resolution_error", outcome.Code)
	require.NotContains(t, outcome.Message, "hunter2")
	require.NotContains(t, outcome.Message, "candidate")
}

// AP stays as it was: its parser still demands the crawl ID, and its payload is unchanged.
func TestActionParserStillRequiresCrawlID(t *testing.T) {
	_, err := Parse(changeInput(t, executeInput, func(m map[string]interface{}) {
		delete(m["resultDelivery"].(map[string]interface{}), "crawlId")
	}))
	require.Error(t, err)
	req, err := Parse([]byte(executeInput))
	require.NoError(t, err)
	raw, err := json.Marshal(integrationDelivery{Delivery: req.ResultDelivery, BaseURL: "https://x"})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"crawlId":"7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c"`)
}
