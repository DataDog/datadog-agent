// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package remotequeries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
)

type queryCheck struct {
	stub.StubCheck
	name, id           string
	supported          bool
	resolve            func(context.Context, string) (string, error)
	execute            func(context.Context, string) (string, error)
	cancel             func(string) error
	resolves, executes atomic.Int32
}

func newQueryCheck(id string) *queryCheck {
	return &queryCheck{name: "postgres", id: id, supported: true}
}
func (c *queryCheck) String() string              { return c.name }
func (c *queryCheck) ID() checkid.ID              { return checkid.ID(c.id) }
func (c *queryCheck) InstanceConfig() string      { panic("remote queries must not read configuration") }
func (c *queryCheck) SupportsRemoteQueries() bool { return c.supported }
func (c *queryCheck) RemoteQuery(ctx context.Context, input string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	var request struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		return "", err
	}
	if request.Operation == "resolve" {
		c.resolves.Add(1)
		if c.resolve != nil {
			return c.resolve(ctx, input)
		}
		return `{"status":"MATCHED"}`, nil
	}
	c.executes.Add(1)
	if c.execute != nil {
		return c.execute(ctx, input)
	}
	return `{"status":"SUCCEEDED"}`, nil
}
func (c *queryCheck) CancelRemoteQuery(id string) error {
	if c.cancel != nil {
		return c.cancel(id)
	}
	return nil
}

type recordingLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *recordingLog) Infof(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}
func (l *recordingLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

type ipcFixture struct {
	svc    *Service
	conf   config.Component
	client ipc.HTTPClient
	server *httptest.Server
	logs   *recordingLog
}

func newIPC(t *testing.T, checks ...check.Check) *ipcFixture {
	t.Helper()
	conf := config.NewMock(t)
	conf.SetInTest("remote_queries.enabled", true)
	conf.SetInTest("private_action_runner.actions_allowlist", []string{"com.datadoghq.remoteaction.queries.execute"})
	logs := &recordingLog{}
	svc := &Service{Config: conf, Checks: func() []check.Check { return checks }, Log: logs}
	mux := http.NewServeMux()
	mux.Handle("/agent/remote-queries", svc)
	server := httptest.NewTLSServer(ipchttp.NewHTTPMiddleware(func(string, ...interface{}) {}, "session-token")(mux))
	t.Cleanup(server.Close)
	transport := server.Client().Transport.(*http.Transport)
	client := ipchttp.NewClient("session-token", transport.TLSClientConfig, conf)
	return &ipcFixture{svc, conf, client, server, logs}
}
func (f *ipcFixture) call(ctx context.Context, input string, trace *TraceContext) (Answer, error) {
	data, err := json.Marshal(WireRequest{Input: json.RawMessage(input), TraceContext: trace})
	if err != nil {
		return Answer{}, err
	}
	body, err := f.client.Post(f.server.URL+"/agent/remote-queries", "application/json", bytes.NewReader(data), ipchttp.WithContext(ctx))
	if err != nil {
		return Answer{}, err
	}
	var answer Answer
	err = json.Unmarshal(body, &answer)
	return answer, err
}
func callIPC(t *testing.T, f *ipcFixture, input string) Answer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answer, err := f.call(ctx, input, nil)
	require.NoError(t, err)
	return answer
}

// ENABLE-01: both the setting and runner action allowlist must permit the request.
func TestENABLE01(t *testing.T) {
	require.False(t, config.NewMock(t).GetBool("remote_queries.enabled"))
	for _, gate := range []string{"setting off", "action not allowed", "allowed wildcard"} {
		for _, input := range []string{resolveInput, executeInput} {
			c := newQueryCheck("candidate-1")
			f := newIPC(t, c)
			switch gate {
			case "setting off":
				f.conf.SetInTest("remote_queries.enabled", false)
			case "action not allowed":
				f.conf.SetInTest("private_action_runner.actions_allowlist", []string{})
			case "allowed wildcard":
				f.conf.SetInTest("private_action_runner.actions_allowlist", []string{"com.datadoghq.remoteaction.queries.*"})
			}
			answer := callIPC(t, f, input)
			if gate == "allowed wildcard" {
				require.Nil(t, answer.Error)
				continue
			}
			if input == resolveInput {
				require.Equal(t, "resolution_error", answer.Status)
				require.Equal(t, "resolution_error", answer.Error.Code)
			} else {
				require.Equal(t, "FAILED", answer.Status)
				require.Equal(t, "disabled", answer.Error.Code)
			}
			require.Zero(t, c.resolves.Load())
			require.Zero(t, c.executes.Load())
		}
	}
}

// ENABLE-02: real IPC middleware and client, over a TLS loopback server.
func TestENABLE02(t *testing.T) {
	f := newIPC(t, newQueryCheck("candidate-1"))
	for _, token := range []string{"", "wrong-token"} {
		transport := f.server.Client().Transport.(*http.Transport)
		bad := ipchttp.NewClient(token, transport.TLSClientConfig, f.conf)
		_, err := bad.Post(f.server.URL+"/agent/remote-queries", "application/json", strings.NewReader(`{"input":`+resolveInput+`}`))
		require.Error(t, err)
	}
	require.Equal(t, "matched", callIPC(t, f, resolveInput).Status)
	request := httptest.NewRequest(http.MethodPost, "https://localhost/agent/remote-queries", strings.NewReader(`{"input":`+resolveInput+`}`))
	request.RemoteAddr = "192.0.2.1:1000"
	writer := httptest.NewRecorder()
	f.svc.ServeHTTP(writer, request)
	require.Equal(t, http.StatusForbidden, writer.Code)
}

// INPUT-06: representative invalid inputs through the operation; detailed values are unit-tested.
func TestINPUT06(t *testing.T) {
	c := newQueryCheck("candidate-1")
	f := newIPC(t, c)
	for _, input := range []string{strings.Replace(resolveInput, `5432`, `0`, 1), strings.Replace(executeInput, `"select secret_value"`, `""`, 1)} {
		answer := callIPC(t, f, input)
		require.NotNil(t, answer.Error)
		if strings.Contains(input, `"resolveOnly":true`) {
			require.Equal(t, "resolution_error", answer.Error.Code)
			require.Contains(t, answer.Error.Message, "target.port")
		} else {
			require.Equal(t, "invalid_request", answer.Error.Code)
			require.Contains(t, answer.Error.Message, "query")
		}
	}
	require.Zero(t, c.resolves.Load())
	require.Zero(t, c.executes.Load())
}

// CHECK-01
func TestCHECK01(t *testing.T) {
	other := newQueryCheck("other")
	other.name = "mysql"
	match := newQueryCheck("candidate-1")
	match.name = "PoStGrEs"
	for _, tc := range []struct {
		checks []check.Check
		status string
	}{{nil, "target_not_found"}, {[]check.Check{other}, "target_not_found"}, {[]check.Check{other, match}, "matched"}} {
		require.Equal(t, tc.status, callIPC(t, newIPC(t, tc.checks...), resolveInput).Status)
	}
	require.Zero(t, other.resolves.Load())
	require.EqualValues(t, 1, match.resolves.Load())
}

// CHECK-02: every candidate receives only resolve's target, never instance configuration.
func TestCHECK02(t *testing.T) {
	var seen atomic.Int32
	var malformed atomic.Bool
	checks := make([]check.Check, 0, 3)
	for _, id := range []string{"one", "two", "three"} {
		c := newQueryCheck(id)
		c.resolve = func(_ context.Context, input string) (string, error) {
			var obj map[string]json.RawMessage
			if json.Unmarshal([]byte(input), &obj) != nil || len(obj) != 2 || string(obj["operation"]) != `"resolve"` || len(obj["target"]) == 0 {
				malformed.Store(true)
			}
			seen.Add(1)
			return `{"status":"FAILED","error":{"code":"target_not_found","message":"not found"}}`, nil
		}
		checks = append(checks, c)
	}
	require.Equal(t, "target_not_found", callIPC(t, newIPC(t, checks...), resolveInput).Status)
	require.EqualValues(t, 3, seen.Load())
	require.False(t, malformed.Load())
}

// CHECK-03: one unanswerable candidate fails closed even beside a match.
func TestCHECK03(t *testing.T) {
	for _, tc := range []struct {
		name, reply                 string
		unsupported, throws, panics bool
	}{
		{"unsupported", "", true, false, false}, {"empty", "", false, false, false}, {"malformed", "not JSON", false, false, false},
		{"multiple", `{"status":"MATCHED"} {"status":"MATCHED"}`, false, false, false},
		{"both", `{"status":"MATCHED","error":{"code":"x","message":"failed"}}`, false, false, false},
		{"failed", `{"status":"FAILED","error":{"code":"target_unavailable","message":"unknown scope"}}`, false, false, false},
		{"unscheduled", `{"status":"FAILED","error":{"code":"cancelled","message":"unscheduled"}}`, false, false, false},
		{"throws", "", false, true, false}, {"panics", "", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := newQueryCheck("bad-candidate")
			bad.supported = !tc.unsupported
			bad.resolve = func(context.Context, string) (string, error) {
				if tc.panics {
					panic("secret config")
				}
				if tc.throws {
					return "", errors.New("secret config")
				}
				return tc.reply, nil
			}
			answer := callIPC(t, newIPC(t, newQueryCheck("match"), bad), resolveInput)
			require.Equal(t, "resolution_error", answer.Status)
			require.Contains(t, answer.Error.Message, "bad-candidate")
			require.NotContains(t, answer.Error.Message, "secret config")
		})
	}
}

// CHECK-05: a successful match creates no ticket; execute reads a new inventory.
func TestCHECK05(t *testing.T) {
	for _, mode := range []string{"removed", "scope changed", "ambiguous", "unanswerable"} {
		t.Run(mode, func(t *testing.T) {
			c := newQueryCheck("old-check")
			f := newIPC(t, c)
			require.Equal(t, "matched", callIPC(t, f, resolveInput).Status)
			code := "target_not_found"
			switch mode {
			case "removed":
				f.svc.Checks = func() []check.Check { return nil }
			case "scope changed":
				c.resolve = func(context.Context, string) (string, error) {
					return `{"status":"FAILED","error":{"code":"target_not_found","message":"scope changed"}}`, nil
				}
			case "ambiguous":
				f.svc.Checks = func() []check.Check { return []check.Check{c, newQueryCheck("added-check")} }
				code = "ambiguous_target"
			case "unanswerable":
				bad := newQueryCheck("added-check")
				bad.supported = false
				f.svc.Checks = func() []check.Check { return []check.Check{c, bad} }
				code = "resolution_error"
			}
			answer := callIPC(t, f, executeInput)
			require.Equal(t, code, answer.Error.Code)
			require.Zero(t, c.executes.Load())
			if mode == "scope changed" {
				require.EqualValues(t, 2, c.resolves.Load())
			}
		})
	}
}

// CHECK-06: match operation carries no query/delivery and never enters execute.
func TestCHECK06(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newIPC(t, c)
	require.Equal(t, "matched", callIPC(t, f, resolveInput).Status)
	require.Zero(t, c.executes.Load())
}

// RUN-01: the execute call receives exactly the annex payload, with no wrapper fields.
func TestRUN01(t *testing.T) {
	c := newQueryCheck("candidate")
	var got string
	c.execute = func(_ context.Context, input string) (string, error) {
		got = input
		return `{"status":"SUCCEEDED"}`, nil
	}
	f := newIPC(t, c)
	trace := &TraceContext{TraceID: "123", ParentID: "456", SamplingPriority: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answer, err := f.call(ctx, executeInput, trace)
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", answer.Status)
	var expected map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(executeInput), &expected))
	delete(expected, "integration")
	delete(expected, "resolveOnly")
	delete(expected, "timeout")
	expected["operation"] = "execute"
	expected["traceContext"] = trace
	raw, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), got)
}

// RUN-04: deadline/caller cancellation discards the answer and signals the run ID.
func TestRUN04(t *testing.T) {
	for _, tc := range []struct{ deadline, lateRegistration bool }{{false, false}, {true, false}, {false, true}} {
		t.Run(strconv.FormatBool(tc.deadline)+"/late="+strconv.FormatBool(tc.lateRegistration), func(t *testing.T) {
			started, finish, cancelled := make(chan struct{}), make(chan struct{}), make(chan string, 1)
			var finishOnce sync.Once
			defer finishOnce.Do(func() { close(finish) })
			var signals atomic.Int32
			c := newQueryCheck("candidate")
			c.execute = func(ctx context.Context, _ string) (string, error) {
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				close(started)
				<-finish
				return `{"status":"SUCCEEDED"}`, nil
			}
			c.cancel = func(id string) error {
				if signals.Add(1) == 1 && tc.lateRegistration {
					return nil
				}
				cancelled <- id
				finishOnce.Do(func() { close(finish) })
				return nil
			}
			f := newIPC(t, c)
			ctx, cancel := context.WithCancel(context.Background())
			if tc.deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 150*time.Millisecond)
			}
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := f.call(ctx, executeInput, nil); done <- err }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("execute did not start")
			}
			if !tc.deadline {
				cancel()
			}
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("caller not released")
			}
			select {
			case id := <-cancelled:
				require.Equal(t, "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c", id)
				if tc.lateRegistration {
					require.GreaterOrEqual(t, signals.Load(), int32(2))
				}
			case <-time.After(3 * time.Second):
				t.Fatal("run not cancelled")
			}
		})
	}
}

// OUT-01: resolution errors identify the candidate, not its configuration.
func TestOUT01(t *testing.T) {
	bad := newQueryCheck("postgres:instance-123")
	bad.supported = false
	answer := callIPC(t, newIPC(t, bad), resolveInput)
	require.Equal(t, "resolution_error", answer.Status)
	require.Equal(t, answer.Status, answer.Error.Code)
	require.Contains(t, answer.Error.Message, string(bad.ID()))
	require.Contains(t, answer.Error.Message, "not supported")
}

// OUT-02 and OUT-03: final execute outcomes, including unchanged integration codes.
func TestOUT02OUT03(t *testing.T) {
	for _, reply := range []string{`{"status":"SUCCEEDED"}`, `{"status":"FAILED","error":{"code":"unsupported_column","message":"expand the column"}}`} {
		c := newQueryCheck("candidate")
		c.execute = func(context.Context, string) (string, error) { return reply, nil }
		answer := callIPC(t, newIPC(t, c), executeInput)
		raw, err := json.Marshal(answer)
		require.NoError(t, err)
		require.JSONEq(t, reply, string(raw))
	}
}

// OUT-04 and OUT-05: malformed final answers and attempts to return data fail closed.
func TestOUT04OUT05(t *testing.T) {
	for _, reply := range []string{"", `[]`, `{"status":"SUCCEEDED"} {"status":"FAILED"}`, `{"status":"SUCCEEDED","error":{"code":"x","message":"failure"}}`, `{"status":"SUCCEEDED","rows":["secret result"]}`, `{"status":"SUCCEEDED","schema":{}}`, `{"status":"SUCCEEDED","pageContents":"secret result"}`} {
		c := newQueryCheck("candidate")
		c.execute = func(context.Context, string) (string, error) { return reply, nil }
		answer := callIPC(t, newIPC(t, c), executeInput)
		require.Equal(t, "FAILED", answer.Status)
		require.Equal(t, "integration_unavailable", answer.Error.Code)
		require.NotContains(t, answer.Error.Message, "secret result")
	}
}

// OBS-02 and OBS-03: one safe summary with IDs and all required measurements.
func TestOBS02OBS03(t *testing.T) {
	c := newQueryCheck("candidate")
	f := newIPC(t, c)
	callIPC(t, f, executeInput)
	lines := f.logs.snapshot()
	require.Len(t, lines, 1)
	for _, field := range []string{`integration="postgres"`, `run_id="7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c"`, `task_id="d25e7f9f-6883-4462-b31e-b449f70c0894"`, `upload_id="upload-1"`, `kind=execute`, `outcome=SUCCEEDED`, `code=`, `duration=`, `candidates=1`, `matched_check="candidate"`} {
		require.Contains(t, lines[0], field)
	}
	require.NotContains(t, lines[0], "secret_value")
	require.NotContains(t, lines[0], "resultDelivery")
}

// SCALE-01: a stuck run does not delay another request's resolve or execute.
func TestSCALE01(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	c := newQueryCheck("candidate")
	var count atomic.Int32
	c.execute = func(context.Context, string) (string, error) {
		if count.Add(1) == 1 {
			close(started)
			<-finish
		}
		return `{"status":"SUCCEEDED"}`, nil
	}
	f := newIPC(t, c)
	done := make(chan struct{})
	go func() { defer close(done); _, _ = f.call(context.Background(), executeInput, nil) }()
	defer func() { close(finish); <-done }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("run not started")
	}
	require.Equal(t, "matched", callIPC(t, f, resolveInput).Status)
	require.Equal(t, "SUCCEEDED", callIPC(t, f, executeInput).Status)
}
