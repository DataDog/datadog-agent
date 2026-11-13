// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package collectorimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries"
)

type lifecycleQueryCheck struct {
	stub.StubCheck
	monitoring, monitorDone                        chan struct{}
	cancelled, release                             chan struct{}
	entered                                        chan struct{}
	cancelOnce, stopOnce, startedOnce, releaseOnce sync.Once
	mu                                             sync.Mutex
	unscheduled                                    bool
	calls                                          sync.WaitGroup
}

func newLifecycleQueryCheck() *lifecycleQueryCheck {
	return &lifecycleQueryCheck{monitoring: make(chan struct{}), monitorDone: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{}), entered: make(chan struct{}, 10)}
}
func (c *lifecycleQueryCheck) finish()                   { c.releaseOnce.Do(func() { close(c.release) }) }
func (*lifecycleQueryCheck) String() string              { return "postgres" }
func (*lifecycleQueryCheck) ID() checkid.ID              { return "postgres:lifecycle" }
func (*lifecycleQueryCheck) Interval() time.Duration     { return time.Hour }
func (*lifecycleQueryCheck) SupportsRemoteQueries() bool { return true }
func (c *lifecycleQueryCheck) Run() error {
	c.startedOnce.Do(func() { close(c.monitoring) })
	<-c.monitorDone
	return nil
}
func (c *lifecycleQueryCheck) Stop() { c.stopOnce.Do(func() { close(c.monitorDone) }) }
func (c *lifecycleQueryCheck) Cancel() {
	c.mu.Lock()
	c.unscheduled = true
	c.mu.Unlock()
	c.cancelOnce.Do(func() { close(c.cancelled) })
	c.calls.Wait()
}
func (c *lifecycleQueryCheck) CancelRemoteQuery(string) error {
	c.cancelOnce.Do(func() { close(c.cancelled) })
	return nil
}
func (c *lifecycleQueryCheck) RemoteQuery(ctx context.Context, input string) (string, error) {
	c.mu.Lock()
	if c.unscheduled {
		c.mu.Unlock()
		return `{"status":"FAILED","error":{"code":"cancelled","message":"unscheduled"}}`, nil
	}
	if ctx.Err() != nil {
		c.mu.Unlock()
		return "", ctx.Err()
	}
	c.calls.Add(1)
	c.mu.Unlock()
	defer c.calls.Done()
	var request struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		return "", err
	}
	if request.Operation == "resolve" {
		return `{"status":"MATCHED"}`, nil
	}
	c.entered <- struct{}{}
	<-c.release
	select {
	case <-c.cancelled:
		return `{"status":"FAILED","error":{"code":"cancelled","message":"unscheduled"}}`, nil
	default:
		return `{"status":"SUCCEEDED"}`, nil
	}
}

type collectorIPC struct {
	client ipc.HTTPClient
	url    string
}

func newCollectorIPC(t *testing.T, c *collectorImpl) *collectorIPC {
	t.Helper()
	c.config.SetInTest("private_action_runner.actions_allowlist", []string{"com.datadoghq.remoteaction.queries.execute"})
	svc := &remotequeries.Service{Config: c.config, Checks: c.GetChecks, Log: c.log}
	server := httptest.NewTLSServer(ipchttp.NewHTTPMiddleware(func(string, ...interface{}) {}, "session-token")(svc))
	t.Cleanup(server.Close)
	transport := server.Client().Transport.(*http.Transport)
	return &collectorIPC{ipchttp.NewClient("session-token", transport.TLSClientConfig, c.config), server.URL}
}
func (f *collectorIPC) call(resolve bool) (remotequeries.Answer, error) {
	input := map[string]interface{}{"integration": "postgres", "target": map[string]interface{}{"database_instance": "orders"}, "resolveOnly": resolve}
	if !resolve {
		input["query"] = "select 1"
		input["includeSchema"] = false
		input["resultDelivery"] = &remotequeries.Delivery{CrawlID: "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c", TaskID: "d25e7f9f-6883-4462-b31e-b449f70c0894", UploadID: "upload", BaseURL: "https://datadoghq.com/intake", Limits: remotequeries.Limits{MaxFileBytes: "1", MaxResultBytes: "1", MaxRowBytes: "1", MaxColumns: "1", MaxSchemaBytes: "1", MaxPages: "1", TimeoutMs: "10000"}}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return remotequeries.Answer{}, err
	}
	body, err := json.Marshal(remotequeries.WireRequest{Input: raw})
	if err != nil {
		return remotequeries.Answer{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := f.client.Post(f.url, "application/json", bytes.NewReader(body), ipchttp.WithContext(ctx))
	if err != nil {
		return remotequeries.Answer{}, err
	}
	var answer remotequeries.Answer
	err = json.Unmarshal(response, &answer)
	return answer, err
}
func waitQueryEvent(t *testing.T, event <-chan struct{}) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(5 * time.Second):
		t.Fatal("check did not reach expected phase")
	}
}

// RUN-02: the real collector's wrapper admits overlapping queries while its
// scheduled monitoring run holds Run's lock.
func TestRUN02(t *testing.T) {
	c := newTestCollector(t, map[string]interface{}{"remote_queries.enabled": true})
	ch := newLifecycleQueryCheck()
	_, err := c.RunCheck(ch)
	require.NoError(t, err)
	defer c.stop(context.Background())
	defer ch.finish()
	waitQueryEvent(t, ch.monitoring)
	f := newCollectorIPC(t, c)
	type result struct {
		answer remotequeries.Answer
		err    error
	}
	done := make(chan result, 2)
	for range 2 {
		go func() { answer, err := f.call(false); done <- result{answer, err} }()
	}
	waitQueryEvent(t, ch.entered)
	waitQueryEvent(t, ch.entered)
	select {
	case <-ch.monitorDone:
		t.Fatal("queries stopped monitoring")
	default:
	}
	ch.finish()
	for range 2 {
		r := <-done
		require.NoError(t, r.err)
		require.Equal(t, "SUCCEEDED", r.answer.Status)
	}
}

// RUN-03: both StopCheck and shutdown close admission and keep teardown blocked
// until an in-progress query has ended. The IPC client sees a cancelled outcome.
func TestRUN03(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "unschedule", true: "shutdown"}[shutdown], func(t *testing.T) {
			c := newTestCollector(t, map[string]interface{}{"remote_queries.enabled": true, "check_cancel_timeout": 5 * time.Second})
			ch := newLifecycleQueryCheck()
			id, err := c.RunCheck(ch)
			require.NoError(t, err)
			defer c.stop(context.Background())
			defer ch.finish()
			f := newCollectorIPC(t, c)
			answerDone := make(chan remotequeries.Answer, 1)
			errDone := make(chan error, 1)
			go func() { answer, err := f.call(false); answerDone <- answer; errDone <- err }()
			waitQueryEvent(t, ch.entered)
			stopped := make(chan error, 1)
			go func() {
				if shutdown {
					stopped <- c.stop(context.Background())
				} else {
					stopped <- c.StopCheck(id)
				}
			}()
			waitQueryEvent(t, ch.cancelled)
			select {
			case <-stopped:
				t.Fatal("check was torn down before query ended")
			default:
			}
			answer, err := f.call(true)
			require.NoError(t, err)
			require.Equal(t, "resolution_error", answer.Status)
			ch.finish()
			require.NoError(t, <-errDone)
			require.Equal(t, "cancelled", (<-answerDone).Error.Code)
			select {
			case err := <-stopped:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("unscheduling did not finish")
			}
		})
	}
}
