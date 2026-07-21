// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package syntheticstestschedulerimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) log.Component {
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(io.Discard, utillog.WarnLvl)
	require.NoError(t, err)
	return l
}

// newStubPoller returns a poller backed by a server that always returns empty tests.
// Used in tests that need a live poller but don't exercise polling behaviour.
func newStubPoller(t *testing.T, logger log.Component) *testPoller {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[]}`))
	}))
	t.Cleanup(server.Close)

	return &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "test-key",
		agentVersion:    "0.0.0-test",
		hostNameService: &mockHostname{},
		log:             logger,
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}
}

func TestTestPoller_FetchTests_ReturnsTests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("DD-API-KEY"))
		assert.Equal(t, "test-hostname", r.URL.Query().Get("agent_hostname"))
		assert.Equal(t, "7.99.0-test", r.URL.Query().Get("agent_version"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[{
			"version":1,"type":"network","subtype":"TCP","org_id":99,"public_id":"t-1","result_id":"r-1",
			"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"SYN"}}
		}]}`))
	}))
	defer server.Close()

	poller := &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "test-api-key",
		agentVersion:    "7.99.0-test",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}

	tests, err := poller.fetchTests(context.Background())
	require.NoError(t, err)
	require.Len(t, tests, 1)
	assert.Equal(t, "t-1", tests[0].PublicID)
	assert.Equal(t, 99, tests[0].OrgID)
	assert.Equal(t, "r-1", tests[0].ResultID)
	assert.Equal(t, "example.com", tests[0].Config.Request.(common.TCPConfigRequest).Host)
}

func TestTestPoller_FetchTests_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[]}`))
	}))
	defer server.Close()

	poller := &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		agentVersion:    "0.0.0",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}

	tests, err := poller.fetchTests(context.Background())
	require.NoError(t, err)
	assert.Empty(t, tests)
}

func TestTestPoller_FetchTests_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	poller := &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		agentVersion:    "0.0.0",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}

	_, err := poller.fetchTests(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestTestPoller_FetchTests_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	poller := &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		agentVersion:    "0.0.0",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}

	_, err := poller.fetchTests(context.Background())
	require.Error(t, err)
}

func TestTestPoller_PollLoop_EnqueuesTests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[{
			"version":1,"type":"network","subtype":"TCP","public_id":"poll-1",
			"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"SYN"}}
		}]}`))
	}))
	defer server.Close()

	now := time.Now()
	poller := &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		agentVersion:    "0.0.0",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       func() time.Time { return now },
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	poller.start(ctx)

	select {
	case testCtx := <-poller.TestsChan:
		assert.Equal(t, "poll-1", testCtx.cfg.PublicID)
		assert.Equal(t, now, testCtx.nextRun)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: no test enqueued by poll loop")
	}

	cancel()
	poller.stop()
}

func TestTestPoller_HealthFlipsAfterNErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	poller := &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		agentVersion:    "0.0.0",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}

	for i := 0; i < maxConsecutiveErrors-1; i++ {
		_, err := poller.fetchTests(context.Background())
		require.Error(t, err)
		poller.markFailure()
	}
	assert.True(t, poller.isHealthy(), "still healthy below threshold")

	_, err := poller.fetchTests(context.Background())
	require.Error(t, err)
	poller.markFailure()
	assert.False(t, poller.isHealthy(), "should be unhealthy at threshold")
}

func TestTestPoller_RecoversAfterSuccess(t *testing.T) {
	var failNext atomic.Bool
	failNext.Store(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failNext.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[]}`))
	}))
	defer server.Close()

	poller := &testPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		agentVersion:    "0.0.0",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}

	for i := 0; i < maxConsecutiveErrors; i++ {
		_, err := poller.fetchTests(context.Background())
		require.Error(t, err)
		poller.markFailure()
	}
	assert.False(t, poller.isHealthy())

	failNext.Store(false)
	_, err := poller.fetchTests(context.Background())
	require.NoError(t, err)
	poller.markSuccess()
	assert.True(t, poller.isHealthy())
}

func TestWorker_PollerPriority(t *testing.T) {
	var mu sync.Mutex
	var processed []string

	tcpPort := 443
	makeCfg := func(id string) common.SyntheticsTestConfig {
		return common.SyntheticsTestConfig{
			PublicID: id,
			Config: struct {
				Assertions []common.Assertion   `json:"assertions"`
				Request    common.ConfigRequest `json:"request"`
			}{
				Request: common.TCPConfigRequest{
					Host:      "example.com",
					Port:      &tcpPort,
					TCPMethod: payload.TCPConfigSYN,
				},
			},
		}
	}

	poller := &testPoller{
		TestsChan: make(chan SyntheticsTestCtx, 10),
		done:      make(chan struct{}),
	}

	l := newTestLogger(t)

	scheduler := &syntheticsTestScheduler{
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 10),
		testPoller:                   poller,
		timeNowFn:                    time.Now,
		log:                          l,
		hostNameService:              &mockHostname{},
		statsdClient:                 &teststatsd.Client{},
		state:                        runningState{tests: map[string]*runningTestState{}},
		traceroute: &tracerouteRunner{fn: func(_ context.Context, _ config.Config) (payload.NetworkPath, error) {
			return payload.NetworkPath{}, nil
		}},
	}
	scheduler.sendResult = func(w *workerResult) (string, error) {
		mu.Lock()
		processed = append(processed, w.testCfg.cfg.PublicID)
		mu.Unlock()
		return "passed", nil
	}

	const n = 3
	for i := 0; i < n; i++ {
		poller.TestsChan <- SyntheticsTestCtx{cfg: makeCfg(fmt.Sprintf("poller-%d", i))}
		scheduler.syntheticsTestProcessingChan <- SyntheticsTestCtx{cfg: makeCfg(fmt.Sprintf("fallback-%d", i))}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go scheduler.runWorker(ctx, 0)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(processed) == 2*n
	}, 5*time.Second, 10*time.Millisecond)

	cancel()

	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		assert.Truef(t, strings.HasPrefix(processed[i], "poller-"),
			"expected poller-delivered test at position %d, got %s", i, processed[i])
	}
	for i := n; i < 2*n; i++ {
		assert.Truef(t, strings.HasPrefix(processed[i], "fallback-"),
			"expected fallback test at position %d, got %s", i, processed[i])
	}
}

func TestWorker_ScheduledPollerTestRefreshesFallbackCache(t *testing.T) {
	l := newTestLogger(t)

	poller := &testPoller{
		TestsChan: make(chan SyntheticsTestCtx, 10),
		done:      make(chan struct{}),
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	scheduler := &syntheticsTestScheduler{
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 10),
		testPoller:                   poller,
		timeNowFn:                    func() time.Time { return now },
		log:                          l,
		hostNameService:              &mockHostname{},
		statsdClient:                 &teststatsd.Client{},
		state:                        runningState{tests: map[string]*runningTestState{}},
		traceroute: &tracerouteRunner{fn: func(_ context.Context, _ config.Config) (payload.NetworkPath, error) {
			return payload.NetworkPath{}, nil
		}},
	}
	scheduler.sendResult = func(_ *workerResult) (string, error) { return "passed", nil }

	port := 443
	cfg := common.SyntheticsTestConfig{
		PublicID: "sched-1",
		RunType:  common.RunTypeScheduled,
		Interval: 30,
		Config: struct {
			Assertions []common.Assertion   `json:"assertions"`
			Request    common.ConfigRequest `json:"request"`
		}{
			Request: common.TCPConfigRequest{Host: "example.com", Port: &port, TCPMethod: payload.TCPConfigSYN},
		},
	}
	poller.TestsChan <- SyntheticsTestCtx{cfg: cfg}

	ctx, cancel := context.WithCancel(context.Background())
	go scheduler.runWorker(ctx, 0)

	require.Eventually(t, func() bool {
		scheduler.state.mu.RLock()
		defer scheduler.state.mu.RUnlock()
		_, ok := scheduler.state.tests["sched-1"]
		return ok
	}, 2*time.Second, 5*time.Millisecond)

	scheduler.state.mu.RLock()
	cached := scheduler.state.tests["sched-1"]
	scheduler.state.mu.RUnlock()
	assert.Equal(t, now.Add(30*time.Second), cached.nextRun)

	cancel()
}
