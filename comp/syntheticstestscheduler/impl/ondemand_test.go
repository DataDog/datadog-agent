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

// newTestLogger returns a logger that discards output, for use in tests.
func newTestLogger(t *testing.T) log.Component {
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(io.Discard, utillog.WarnLvl)
	require.NoError(t, err)
	return l
}

// newTestPoller returns a poller backed by a server that always returns empty tests.
// Used in tests that need a live poller but don't exercise on-demand behaviour.
func newTestPoller(t *testing.T, logger log.Component) *onDemandPoller {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[]}`))
	}))
	t.Cleanup(server.Close)

	return &onDemandPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "test-key",
		hostNameService: &mockHostname{},
		log:             logger,
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
	}
}

func TestOnDemandPoller_FetchTests_ReturnsTests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("DD-API-KEY"))
		assert.Equal(t, "test-hostname", r.URL.Query().Get("agent_hostname"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[{
			"version":1,"type":"network","subtype":"TCP","org_id":99,"public_id":"od-1","result_id":"od-result-1",
			"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"SYN"}}
		}]}`))
	}))
	defer server.Close()

	poller := &onDemandPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "test-api-key",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
	}

	tests, err := poller.fetchTests(context.Background())
	require.NoError(t, err)
	require.Len(t, tests, 1)
	assert.Equal(t, "od-1", tests[0].PublicID)
	assert.Equal(t, 99, tests[0].OrgID)
	assert.Equal(t, "od-result-1", tests[0].ResultID)
	assert.Equal(t, "example.com", tests[0].Config.Request.(common.TCPConfigRequest).Host)
}

func TestOnDemandPoller_FetchTests_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[]}`))
	}))
	defer server.Close()

	poller := &onDemandPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
	}

	tests, err := poller.fetchTests(context.Background())
	require.NoError(t, err)
	assert.Empty(t, tests)
}

func TestOnDemandPoller_FetchTests_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	poller := &onDemandPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
	}

	_, err := poller.fetchTests(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestOnDemandPoller_FetchTests_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	poller := &onDemandPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       time.Now,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
	}

	_, err := poller.fetchTests(context.Background())
	require.Error(t, err)
}

func TestOnDemandPoller_PollLoop_EnqueuesTests(t *testing.T) {
	serverCalled := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case serverCalled <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tests":[{
			"version":1,"type":"network","subtype":"TCP","public_id":"od-poll-1",
			"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"SYN"}}
		}]}`))
	}))
	defer server.Close()

	now := time.Now()
	poller := &onDemandPoller{
		httpClient:      server.Client(),
		endpoint:        server.URL,
		apiKey:          "key",
		hostNameService: &mockHostname{},
		log:             newTestLogger(t),
		timeNowFn:       func() time.Time { return now },
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	poller.start(ctx)

	select {
	case testCtx := <-poller.TestsChan:
		assert.Equal(t, "od-poll-1", testCtx.cfg.PublicID)
		assert.Equal(t, now, testCtx.nextRun)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: no test enqueued by poll loop")
	}

	cancel()
	poller.stop()
}

func TestWorker_OnDemandPriority(t *testing.T) {
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

	poller := &onDemandPoller{
		TestsChan: make(chan SyntheticsTestCtx, 10),
		done:      make(chan struct{}),
	}

	l := newTestLogger(t)

	scheduler := &syntheticsTestScheduler{
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 10),
		onDemandPoller:               poller,
		timeNowFn:                    time.Now,
		log:                          l,
		hostNameService:              &mockHostname{},
		statsdClient:                 &teststatsd.Client{},
		traceroute:                   &tracerouteRunner{fn: func(_ context.Context, _ config.Config) (payload.NetworkPath, error) {
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
		poller.TestsChan <- SyntheticsTestCtx{cfg: makeCfg(fmt.Sprintf("ondemand-%d", i))}
		scheduler.syntheticsTestProcessingChan <- SyntheticsTestCtx{cfg: makeCfg(fmt.Sprintf("scheduled-%d", i))}
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
		assert.Truef(t, strings.HasPrefix(processed[i], "ondemand-"),
			"expected on-demand test at position %d, got %s", i, processed[i])
	}
	for i := n; i < 2*n; i++ {
		assert.Truef(t, strings.HasPrefix(processed[i], "scheduled-"),
			"expected scheduled test at position %d, got %s", i, processed[i])
	}
}
