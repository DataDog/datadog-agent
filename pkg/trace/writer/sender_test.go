// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const testAPIKey = "123"

func TestMain(m *testing.M) {
	log.SetLogger(log.NoopLogger)
	os.Exit(m.Run())
}

func TestMaxConns(t *testing.T) {
	for tn, tc := range map[string]struct {
		endpoints []*config.Endpoint
		climit    int
		maxConns  int
	}{
		"single-non-mrf": {
			[]*config.Endpoint{{IsMRF: false}},
			5,
			5,
		},
		"multiple-non-mrf": {
			[]*config.Endpoint{{IsMRF: false}, {IsMRF: false}},
			5,
			2,
		},
		"multiple-with-mrf": {
			[]*config.Endpoint{{IsMRF: false}, {IsMRF: true}},
			5,
			5,
		},
		"single-mrf": {
			[]*config.Endpoint{{IsMRF: true}},
			5,
			5,
		},
	} {
		assert.Equal(t, tc.maxConns, maxConns(tc.climit, tc.endpoints), "%s", tn)
	}
}

func TestIsRetriable(t *testing.T) {
	for code, want := range map[int]bool{
		400: false,
		403: true,
		404: false,
		408: true,
		409: false,
		429: true,
		500: true,
		503: true,
		505: true,
		200: false,
		204: false,
		306: false,
		101: false,
	} {
		assert.Equal(t, isRetriable(code), want)
	}
}

func TestSender(t *testing.T) {
	t.Run("accept", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()

		s, err := newTestSender(server.URL)
		assert.NoError(err)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20; i++ {
			s.Push(expectResponses(200))
		}
		s.Stop()

		assert.Equal(20, server.Total(), "total")
		assert.Equal(20, server.Accepted(), "accepted")
		assert.Equal(0, server.Retried(), "retry")
		assert.Equal(0, server.Failed(), "failed")
	})

	t.Run("peak", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServerWithLatency(50 * time.Millisecond)
		defer server.Close()

		s, err := newTestSender(server.URL)
		assert.NoError(err)
		for i := 0; i < s.cfg.maxConns*2; i++ {
			// we have to sleep for a bit to yield to the receiver, otherwise
			// the channel will get immediately full.
			time.Sleep(time.Millisecond)
			s.Push(expectResponses(200))
		}
		s.Stop()

		assert.True(server.Peak() <= s.cfg.maxConns)
		assert.Equal(s.cfg.maxConns*2, server.Total(), "total")
		assert.Equal(s.cfg.maxConns*2, server.Accepted(), "accepted")
		assert.Equal(0, server.Retried(), "retry")
		assert.Equal(0, server.Failed(), "failed")
	})

	t.Run("failed", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()

		s, err := newTestSender(server.URL)
		assert.NoError(err)
		for i := 0; i < 20; i++ {
			s.Push(expectResponses(404))
		}
		s.Stop()

		assert.Equal(20, server.Total(), "total")
		assert.Equal(0, server.Accepted(), "accepted")
		assert.Equal(0, server.Retried(), "retry")
		assert.Equal(20, server.Failed(), "failed")
	})

	t.Run("retry", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer func(old func(int) time.Duration) { backoffDuration = old }(backoffDuration)
		var backoffCalls []int
		backoffDuration = func(d int) time.Duration {
			backoffCalls = append(backoffCalls, d)
			return time.Nanosecond
		}

		s, err := newTestSender(server.URL)
		assert.NoError(err)
		s.Push(expectResponses(503, 408, 200))
		s.Stop()

		assert.Equal([]int{0, 1, 2}, backoffCalls)
		assert.Equal(3, server.Total(), "total")
		assert.Equal(2, server.Retried(), "retry")
		assert.Equal(1, server.Accepted(), "accepted")
	})

	t.Run("many", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer useBackoffDuration(time.Millisecond)()

		s, err := newTestSender(server.URL)
		assert.NoError(err)
		s.Push(expectResponses(503, 503, 200))
		for i := 0; i < 20; i++ {
			s.Push(expectResponses(404))
		}

		s.Stop()

		assert.Equal(23, server.Total(), "total")
		assert.Equal(2, server.Retried(), "retry")
		assert.Equal(1, server.Accepted(), "accepted")
		assert.Equal(20, server.Failed(), "failed")
	})

	t.Run("headers", func(t *testing.T) {
		assert := assert.New(t)
		var wg sync.WaitGroup
		wg.Add(1)
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			assert.Equal(testAPIKey, req.Header.Get(headerAPIKey))
			assert.Equal("testUserAgent", req.Header.Get(headerUserAgent))
			wg.Done()
		}))
		defer server.Close()
		s, err := newTestSender(server.URL)
		assert.NoError(err)
		s.Push(expectResponses(http.StatusOK))
		s.Stop()
		wg.Wait()
	})

	t.Run("events", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer useBackoffDuration(0)()

		var recorder mockRecorder

		s, err := newTestSender(server.URL)
		assert.NoError(err)
		s.cfg.recorder = &recorder

		// push a couple of payloads
		start := time.Now()
		s.Push(expectResponses(503, 503, 200))
		s.Push(expectResponses(200))
		s.Push(expectResponses(200))
		for i := 0; i < 4; i++ {
			s.Push(expectResponses(404))
		}
		s.Stop()

		retried := recorder.data(eventTypeRetry)
		assert.Equal(2, len(retried))
		for i := 0; i < 2; i++ {
			assert.True(retried[i].bytes > len("|503,503,200"))
			assert.Equal(`server responded with "503 Service Unavailable"`, retried[i].err.(*retriableError).err.Error())
			assert.Equal(1, retried[i].count)
			assert.True(time.Since(start)-retried[i].duration < time.Second)
		}

		sent := recorder.data(eventTypeSent)
		assert.Equal(3, len(sent))
		for i := 0; i < 3; i++ {
			assert.True(sent[i].bytes > len("|404"))
			assert.NoError(sent[i].err)
			assert.Equal(1, sent[i].count)
			assert.True(time.Since(start)-sent[i].duration < time.Second)
		}

		failed := recorder.data(eventTypeRejected)
		assert.Equal(4, len(failed))
		for i := 0; i < 4; i++ {
			assert.True(failed[i].bytes > len("|404"))
			assert.Equal("404 Not Found", failed[i].err.Error())
			assert.Equal(1, failed[i].count)
			assert.True(time.Since(start)-failed[i].duration < time.Second)
		}
	})

	t.Run("mrf", func(t *testing.T) {
		assert := assert.New(t)
		servers := []*testServer{
			newTestServer(),
			newTestServer(),
			newTestServer(),
		}
		for _, server := range servers {
			defer server.Close()
		}

		senders := make([]*sender, 3)
		for i := range 3 {
			s, err := newTestSender(servers[i].URL)
			assert.NoError(err)
			senders[i] = s
		}
		// Enable and failover MRF on s1, enable and not failover on s2, disabled on s3
		senders[0].cfg.isMRF = true
		senders[0].cfg.MRFFailoverAPM = func() bool { return true }
		senders[1].cfg.isMRF = true
		senders[1].cfg.MRFFailoverAPM = func() bool { return false }

		assert.True(senders[0].isEnabled())
		assert.False(senders[1].isEnabled())
		assert.True(senders[2].isEnabled())

		for i := 0; i < 20; i++ {
			sendPayloads(senders, expectResponses(404), false)
		}
		for _, s := range senders {
			s.Stop()
		}

		// Server 1 receives payloads
		assert.Equal(20, servers[0].Total(), "total")
		assert.Equal(0, servers[0].Accepted(), "accepted")
		assert.Equal(0, servers[0].Retried(), "retry")
		assert.Equal(20, servers[0].Failed(), "failed")

		// Server 2 doesn't
		assert.Equal(0, servers[1].Total(), "total")
		assert.Equal(0, servers[1].Accepted(), "accepted")
		assert.Equal(0, servers[1].Retried(), "retry")
		assert.Equal(0, servers[1].Failed(), "failed")

		// Server 3 receives payloads
		assert.Equal(20, servers[2].Total(), "total")
		assert.Equal(0, servers[2].Accepted(), "accepted")
		assert.Equal(0, servers[2].Retried(), "retry")
		assert.Equal(20, servers[2].Failed(), "failed")
	})

	t.Run("403_secrets_refresh_fn", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()

		s, err := newTestSender(server.URL)
		assert.NoError(err)

		callbackInvoked := false
		s.apiKeyManager.refreshFn = func() (string, error) {
			callbackInvoked = true
			return "secrets refreshed", nil
		}
		s.apiKeyManager.throttleInterval = 100 * time.Millisecond

		s.Push(expectResponses(403))
		s.Stop()

		assert.True(callbackInvoked, "secrets refresh callback should have been invoked on 403")
	})
	t.Run("403_secrets_refresh_fn_nil", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()

		s, err := newTestSender(server.URL)
		assert.NoError(err)

		assert.NotPanics(func() {
			s.Push(expectResponses(403))
			s.Stop()
		})
	})

	t.Run("403_retries_with_backoff", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer useBackoffDuration(time.Nanosecond)()

		s, err := newTestSender(server.URL)
		assert.NoError(err)

		callbackInvoked := false
		s.apiKeyManager.refreshFn = func() (string, error) {
			callbackInvoked = true
			return "secrets refreshed", nil
		}
		s.apiKeyManager.throttleInterval = 100 * time.Millisecond

		assert.NoError(err)
		s.Push(expectResponses(403, 403, 200))
		s.Stop()

		assert.Equal(3, server.Total(), "should have made 3 requests")
		assert.Equal(2, server.Retried(), "should have retried twice")
		assert.Equal(1, server.Accepted(), "should have succeeded once")
		assert.True(callbackInvoked, "secrets refresh callback should have been invoked on 403")
	})

	t.Run("403_throttles_refresh", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer useBackoffDuration(time.Nanosecond)()

		s, err := newTestSender(server.URL)
		assert.NoError(err)

		callCount := 0
		s.apiKeyManager.refreshFn = func() (string, error) {
			callCount++
			return "secrets refreshed", nil
		}
		s.apiKeyManager.throttleInterval = 100 * time.Millisecond

		// First 403 should trigger refresh
		s.Push(expectResponses(403))
		s.WaitForInflight()
		assert.Equal(1, callCount, "first 403 should trigger refresh")

		s.Push(expectResponses(403))
		s.WaitForInflight()
		assert.Equal(1, callCount, "second 403 should be throttled")

		// Wait for throttle interval to expire
		time.Sleep(110 * time.Millisecond)

		s.Push(expectResponses(403))
		s.WaitForInflight()
		assert.Equal(2, callCount, "third 403 after throttle should trigger refresh")

		s.Stop()
	})
}

func TestPayload(t *testing.T) {
	expectBody := bytes.NewBufferString("body")
	bodyLength := strconv.Itoa(expectBody.Len())

	t.Run("headers", func(t *testing.T) {
		assert := assert.New(t)
		p := newPayload(map[string]string{
			"k1": "v1",
			"k2": "v2",
		})
		p.body = expectBody
		u, err := url.Parse("http://whatever")
		assert.NoError(err)
		req, err := p.httpRequest(u)
		assert.NoError(err)
		assert.Len(req.Header, 3)
		assert.Equal(req.Header.Get("Content-Length"), bodyLength)
		assert.Equal("v1", req.Header.Get("k1"))
		assert.Equal("v2", req.Header.Get("k2"))
	})

	t.Run("httpRequest", func(t *testing.T) {
		assert := assert.New(t)
		p := newPayload(map[string]string{"DD-Api-Key": testAPIKey})
		p.body = expectBody
		url, err := url.Parse("http://localhost/my/path")
		if err != nil {
			t.Fatal(err)
		}
		req, err := p.httpRequest(url)
		assert.NoError(err)
		assert.Equal(http.MethodPost, req.Method)
		assert.Equal("/my/path", req.URL.Path)
		assert.Equal("4", req.Header.Get("Content-Length"))
		assert.Equal(testAPIKey, req.Header.Get("DD-Api-Key"))
		slurp, err := io.ReadAll(req.Body)
		assert.NoError(err)
		req.Body.Close()
		assert.Equal(expectBody.Bytes(), slurp)
	})
}

// useBackoffDuration replaces the current backoff duration with d and returns a
// function which restores it.
func useBackoffDuration(d time.Duration) func() {
	old := backoffDuration
	backoffDuration = func(_ int) time.Duration { return d }
	return func() { backoffDuration = old }
}

// mockRecorder is a mock eventRecorder which records all calls to recordEvent.
type mockRecorder struct {
	mu                             sync.RWMutex
	retry, sent, dropped, rejected []*eventData
}

// data returns all call data for the given eventType.
func (r *mockRecorder) data(t eventType) []*eventData {
	r.mu.RLock()
	defer r.mu.RUnlock()
	switch t {
	case eventTypeRetry:
		return r.retry
	case eventTypeSent:
		return r.sent
	case eventTypeDropped:
		return r.dropped
	case eventTypeRejected:
		return r.rejected
	default:
		panic("unknown event")
	}
}

// recordEvent implements eventRecorder.
func (r *mockRecorder) recordEvent(t eventType, data *eventData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch t {
	case eventTypeRetry:
		r.retry = append(r.retry, data)
	case eventTypeSent:
		r.sent = append(r.sent, data)
	case eventTypeDropped:
		r.dropped = append(r.dropped, data)
	case eventTypeRejected:
		r.rejected = append(r.rejected, data)
	}
}

func newTestSender(serverURL string) (*sender, error) {
	url, err := url.Parse(serverURL + "/")
	if err != nil {
		return nil, err
	}
	cfg := config.New()
	cfg.ConnectionResetInterval = 0
	scfg := &senderConfig{
		client:     cfg.NewHTTPClient(),
		url:        url,
		maxConns:   100,
		maxQueued:  40,
		maxRetries: 4,
		userAgent:  "testUserAgent",
	}
	apiKeyManager := &apiKeyManager{
		apiKey: testAPIKey,
	}
	statsd := &statsd.NoOpClient{}
	return newSender(scfg, apiKeyManager, statsd), nil
}
