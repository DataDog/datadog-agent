// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func makeURLs(t *testing.T, ss ...string) []*url.URL {
	var urls []*url.URL
	for _, s := range ss {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatalf("Cannot parse url: %s", s)
		}
		urls = append(urls, u)
	}
	return urls
}

func TestProfileProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if body := string(slurp); body != "body" {
			t.Fatalf("invalid request body: %q", body)
		}
		if v := req.Header.Get("DD-API-KEY"); v != "123" {
			t.Fatalf("got invalid API key: %q", v)
		}
		if v := req.Header.Get("X-Datadog-Additional-Tags"); v != "key:val" {
			t.Fatalf("got invalid X-Datadog-Additional-Tags: %q", v)
		}
		_, err = w.Write([]byte("OK"))
		if err != nil {
			t.Fatal(err)
		}
	}))
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", "dummy.com/path", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := &config.AgentConfig{ContainerIDFromOriginInfo: config.NoopContainerIDFromOriginInfoFunc}
	newProfileProxy(c, []*url.URL{u}, []string{"123"}, "key:val", &statsd.NoOpClient{}).ServeHTTP(rec, req)
	result := rec.Result()
	slurp, err := io.ReadAll(result.Body)
	result.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(slurp) != "OK" {
		t.Fatal("did not proxy")
	}
}

func TestProfilingEndpoints(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test_api_key"
		cfg.ProfilingProxy = config.ProfilingProxyConfig{
			DDURL: "https://intake.profile.datadoghq.fr/api/v2/profile",
		}
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.fr/api/v2/profile"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("site", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test_api_key"
		cfg.Site = "datadoghq.eu"
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.eu/api/v2/profile"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("default", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test_api_key"
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.com/api/v2/profile"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("multiple", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key_0"
		cfg.ProfilingProxy = config.ProfilingProxyConfig{
			DDURL: "https://intake.profile.datadoghq.jp/api/v2/profile",
			AdditionalEndpoints: map[string][]string{
				"https://ddstaging.datadoghq.com": {"api_key_1", "api_key_2"},
				"https://dd.datad0g.com":          {"api_key_3"},
			},
		}
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		expectedURLs := makeURLs(t,
			"https://intake.profile.datadoghq.jp/api/v2/profile",
			"https://ddstaging.datadoghq.com",
			"https://ddstaging.datadoghq.com",
			"https://dd.datad0g.com",
		)
		expectedKeys := []string{"api_key_0", "api_key_1", "api_key_2", "api_key_3"}

		// Because we're using a map to mock the config we can't assert on the
		// order of the endpoints. We check the main endpoints separately.
		assert.Equal(t, urls[0], expectedURLs[0], "The main endpoint should be the first in the slice")
		assert.Equal(t, keys[0], expectedKeys[0], "The main api key should be the first in the slice")

		assert.ElementsMatch(t, urls, expectedURLs, "All urls from the config should be returned")
		assert.ElementsMatch(t, keys, keys, "All keys from the config should be returned")

		// check that we have the correct pairing between urls and api keys
		for i := range keys {
			for j := range expectedKeys {
				if keys[i] == expectedKeys[j] {
					assert.Equal(t, urls[i], expectedURLs[j])
				}
			}
		}
	})
}

func TestProfileProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			tags := strings.Split(v, ",")
			m := make(map[string]string)
			for _, tag := range tags {
				kv := strings.Split(tag, ":")
				if strings.Contains(kv[0], "orchestrator") {
					t.Fatalf("non-fargate environment shouldn't contain '%s' tag : %q", kv[0], v)
				}
				m[kv[0]] = kv[1]
			}
			for _, tag := range []string{"host", "default_env", "agent_version"} {
				if _, ok := m[tag]; !ok {
					t.Fatalf("invalid X-Datadog-Additional-Tags header, should contain '%s': %q", tag, v)
				}
			}
			called = true
		}))
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("proxy_code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		req, _ := http.NewRequest("POST", "/some/path", nil)
		receiver := newTestReceiverFromConfig(conf)
		rec := httptest.NewRecorder()
		receiver.profileProxyHandler().ServeHTTP(rec, req)
		resp := rec.Result()
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ok_fargate", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			if !strings.Contains(v, "orchestrator:fargate_orchestrator") {
				t.Fatalf("invalid X-Datadog-Additional-Tags header, fargate env should contain '%s' tag: %q", "orchestrator", v)
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.Hostname = "myhost"
		conf.FargateOrchestrator = "orchestrator"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("error", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "asd:\r\n"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r := newTestReceiverFromConfig(conf)
		r.profileProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		slurp, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "error parsing main profiling intake URL") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})

	t.Run("multiple_targets", func(t *testing.T) {
		called := make(map[string]bool)
		handler := func(_ http.ResponseWriter, req *http.Request) {
			called[fmt.Sprintf("http://%s|%s", req.Host, req.Header.Get("DD-API-KEY"))] = true
		}
		srv1 := httptest.NewServer(http.HandlerFunc(handler))
		srv2 := httptest.NewServer(http.HandlerFunc(handler))
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key_0"
		cfg.Hostname = "myhost"
		cfg.ProfilingProxy = config.ProfilingProxyConfig{
			DDURL: srv1.URL,
			AdditionalEndpoints: map[string][]string{
				srv2.URL: {"dummy_api_key_1", "dummy_api_key_2"},
				// this should be ignored
				"foobar": {"invalid_url"},
			},
		}

		req, err := http.NewRequest("POST", "/some/path", bytes.NewBuffer([]byte("abc")))
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(cfg)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)

		expected := map[string]bool{
			srv1.URL + "|api_key_0":       true,
			srv2.URL + "|dummy_api_key_1": true,
			srv2.URL + "|dummy_api_key_2": true,
		}
		assert.Equal(t, expected, called, "The request should be proxied to all valid targets")
	})

	t.Run("lambda_function", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			if !strings.Contains(v, "functionname:my-function-name") || !strings.Contains(v, "_dd.origin:lambda") {
				t.Fatalf("invalid X-Datadog-Additional-Tags header, fargate env should contain '%s' tag: %q", "orchestrator", v)
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.LambdaFunctionName = "my-function-name"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("azure_container_app", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			tags := strings.Split(v, ",")
			m := make(map[string]string)
			for _, tag := range tags {
				kv := strings.Split(tag, ":")
				m[kv[0]] = kv[1]
			}
			for _, tag := range []string{"subscription_id", "resource_group", "resource_id", "aca.subscription.id", "aca.resource.group", "aca.resource.id", "aca.replica.name"} {
				if _, ok := m[tag]; !ok {
					t.Fatalf("invalid X-Datadog-Additional-Tags header, should contain '%s': %q", tag, v)
				}
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.AzureContainerAppTags = ",subscription_id:123,resource_group:test-rg,resource_id:456,aca.subscription.id:123,aca.resource.group:test-rg,aca.resource.id:456,aca.replica.name:test-replica"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})
}

// mockRoundTripper allows controlling the behavior of HTTP requests for testing
type mockRoundTripper struct {
	responses []mockResponse
	callCount int
	calls     []mockCall
}

type mockResponse struct {
	resp *http.Response
	err  error
}

type mockCall struct {
	url    string
	apiKey string
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("unexpected call %d, only %d responses configured", m.callCount, len(m.responses))
	}

	// Record the call
	m.calls = append(m.calls, mockCall{
		url:    req.URL.String(),
		apiKey: req.Header.Get("DD-API-KEY"),
	})

	resp := m.responses[m.callCount]
	m.callCount++
	return resp.resp, resp.err
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      mockNetError{timeout: true},
			expected: true,
		},
		{
			name:     "temporary error (non-timeout)",
			err:      mockNetError{timeout: false},
			expected: true,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some generic error"),
			expected: false,
		},
		{
			name:     "url error with retryable nested error",
			err:      &url.Error{Op: "Get", URL: "http://example.com", Err: mockNetError{timeout: true}},
			expected: true,
		},
		{
			name:     "url error with non-retryable nested error",
			err:      &url.Error{Op: "Get", URL: "http://example.com", Err: fmt.Errorf("non-retryable")},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateRetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		attempt  int
		expected time.Duration
	}{
		{
			name:     "attempt 0",
			attempt:  0,
			expected: 100 * time.Millisecond,
		},
		{
			name:     "attempt 1",
			attempt:  1,
			expected: 200 * time.Millisecond,
		},
		{
			name:     "attempt 2",
			attempt:  2,
			expected: 400 * time.Millisecond,
		},
		{
			name:     "attempt 3",
			attempt:  3,
			expected: 800 * time.Millisecond,
		},
		{
			name:     "attempt 4",
			attempt:  4,
			expected: 1600 * time.Millisecond,
		},
		{
			name:     "attempt 5 (capped)",
			attempt:  5,
			expected: 2 * time.Second,
		},
		{
			name:     "attempt 10 (capped)",
			attempt:  10,
			expected: 2 * time.Second,
		},
		{
			name:     "negative attempt",
			attempt:  -1,
			expected: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateRetryDelay(tt.attempt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMultiTransportRetry(t *testing.T) {
	t.Run("single_target_success_first_attempt", func(t *testing.T) {
		mockRT := &mockRoundTripper{
			responses: []mockResponse{
				{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))}, err: nil},
			},
		}

		transport := &multiTransport{
			rt:      mockRT,
			targets: makeURLs(t, "http://example.com"),
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader("test-body"))
		resp, err := transport.RoundTrip(req)
		if err == nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 1, mockRT.callCount)
	})

	t.Run("single_target_retry_on_timeout", func(t *testing.T) {
		mockRT := &mockRoundTripper{
			responses: []mockResponse{
				{resp: nil, err: mockNetError{timeout: true}},
				{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))}, err: nil},
			},
		}

		transport := &multiTransport{
			rt:      mockRT,
			targets: makeURLs(t, "http://example.com"),
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader("test-body"))
		resp, err := transport.RoundTrip(req)
		if err == nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 2, mockRT.callCount, "Should retry once")
	})

	t.Run("single_target_no_retry_on_non_retryable_error", func(t *testing.T) {
		mockRT := &mockRoundTripper{
			responses: []mockResponse{
				{resp: nil, err: fmt.Errorf("non-retryable error")},
			},
		}

		transport := &multiTransport{
			rt:      mockRT,
			targets: makeURLs(t, "http://example.com"),
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader("test-body"))
		resp, err := transport.RoundTrip(req)
		if resp != nil {
			defer resp.Body.Close()
		}

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 1, mockRT.callCount, "Should not retry")
		assert.Contains(t, err.Error(), "non-retryable error")
	})

	t.Run("single_target_exhaust_retries", func(t *testing.T) {
		mockRT := &mockRoundTripper{
			responses: []mockResponse{
				{resp: nil, err: mockNetError{timeout: true}},
				{resp: nil, err: mockNetError{timeout: true}},
			},
		}

		transport := &multiTransport{
			rt:      mockRT,
			targets: makeURLs(t, "http://example.com"),
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader("test-body"))
		resp, err := transport.RoundTrip(req)
		if resp != nil {
			defer resp.Body.Close()
		}

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 2, mockRT.callCount, "Should retry maxRetryAttempts (2) times")
	})

	t.Run("multiple_targets_main_endpoint_retries", func(t *testing.T) {
		mockRT := &mockRoundTripper{
			responses: []mockResponse{
				// Main endpoint fails then succeeds
				{resp: nil, err: mockNetError{timeout: true}},
				{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))}, err: nil},
				// Additional endpoint succeeds immediately (runs asynchronously)
				{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))}, err: nil},
			},
		}

		transport := &multiTransport{
			rt:      mockRT,
			targets: makeURLs(t, "http://main.example.com", "http://additional.example.com"),
			keys:    []string{"main-key", "additional-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader("test-body"))
		resp, err := transport.RoundTrip(req)
		if err == nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		// Main endpoint should have been retried once, additional endpoint called once
		// Total calls might be 2 or 3 depending on goroutine timing, but main endpoint should succeed
	})
}

func TestMultiTransportBackwardsCompatibility(t *testing.T) {
	t.Run("status_code_conversion", func(t *testing.T) {
		mockRT := &mockRoundTripper{
			responses: []mockResponse{
				{resp: &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader("Accepted"))}, err: nil},
			},
		}

		transport := &multiTransport{
			rt:      mockRT,
			targets: makeURLs(t, "http://example.com"),
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader("test-body"))
		resp, err := transport.RoundTrip(req)
		if err == nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "202 should be converted to 200")
		assert.Equal(t, "OK", resp.Status, "Status text should be OK")
	})
}

func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{
			name:       "200 OK - not retryable",
			statusCode: http.StatusOK,
			expected:   false,
		},
		{
			name:       "400 Bad Request - not retryable",
			statusCode: http.StatusBadRequest,
			expected:   false,
		},
		{
			name:       "404 Not Found - not retryable",
			statusCode: http.StatusNotFound,
			expected:   false,
		},
		{
			name:       "500 Internal Server Error - not retryable",
			statusCode: http.StatusInternalServerError,
			expected:   false,
		},
		{
			name:       "502 Bad Gateway - retryable",
			statusCode: http.StatusBadGateway,
			expected:   true,
		},
		{
			name:       "503 Service Unavailable - retryable",
			statusCode: http.StatusServiceUnavailable,
			expected:   true,
		},
		{
			name:       "504 Gateway Timeout - retryable",
			statusCode: http.StatusGatewayTimeout,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableStatusCode(tt.statusCode)
			assert.Equal(t, tt.expected, result, "Expected %v for status code %d", tt.expected, tt.statusCode)
		})
	}
}

func TestMultiTransportStatusCodeRetry(t *testing.T) {
	t.Run("502_retry_success", func(t *testing.T) {
		// Mock transport that returns 502 twice, then succeeds
		mockTransport := &mockRoundTripper{
			responses: []mockResponse{
				{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("bad gateway"))}, err: nil},
				{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("bad gateway"))}, err: nil},
				{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("success"))}, err: nil},
			},
		}

		u, _ := url.Parse("http://example.com")
		mt := &multiTransport{
			rt:      mockTransport,
			targets: []*url.URL{u},
			keys:    []string{"test-key"},
		}

		// Use nil body for single target to avoid body consumption issues
		req, _ := http.NewRequest("POST", "http://example.com", nil)
		resp, err := mt.RoundTrip(req)
		if err == nil && resp != nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.NotNil(t, resp, "Response should not be nil")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, 3, mockTransport.callCount, "Should have made 3 attempts")
	})

	t.Run("502_retry_exhausted", func(t *testing.T) {
		// Mock transport that always returns 502
		mockTransport := &mockRoundTripper{
			responses: []mockResponse{
				{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("bad gateway"))}, err: nil},
				{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("bad gateway"))}, err: nil},
				{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("bad gateway"))}, err: nil},
			},
		}

		u, _ := url.Parse("http://example.com")
		mt := &multiTransport{
			rt:      mockTransport,
			targets: []*url.URL{u},
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", nil)
		resp, err := mt.RoundTrip(req)
		if err == nil && resp != nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err, "Should not return connection error for HTTP response")
		assert.NotNil(t, resp, "Response should not be nil")
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode, "Should return final 502 response")
		assert.Equal(t, 3, mockTransport.callCount, "Should have made 3 attempts (maxRetryAttempts)")
	})

	t.Run("503_retry_success", func(t *testing.T) {
		// Test that 503 Service Unavailable is also retried
		mockTransport := &mockRoundTripper{
			responses: []mockResponse{
				{resp: &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("unavailable"))}, err: nil},
				{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("success"))}, err: nil},
			},
		}

		u, _ := url.Parse("http://example.com")
		mt := &multiTransport{
			rt:      mockTransport,
			targets: []*url.URL{u},
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", nil)
		resp, err := mt.RoundTrip(req)
		if err == nil && resp != nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.NotNil(t, resp, "Response should not be nil")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, 2, mockTransport.callCount, "Should have made 2 attempts (1 retry)")
	})

	t.Run("404_no_retry", func(t *testing.T) {
		// Test that 404 is NOT retried
		mockTransport := &mockRoundTripper{
			responses: []mockResponse{
				{resp: &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, err: nil},
			},
		}

		u, _ := url.Parse("http://example.com")
		mt := &multiTransport{
			rt:      mockTransport,
			targets: []*url.URL{u},
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", nil)
		resp, err := mt.RoundTrip(req)
		if err == nil && resp != nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.NotNil(t, resp, "Response should not be nil")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Equal(t, 1, mockTransport.callCount, "Should have made only 1 attempt (no retry)")
	})

	t.Run("500_no_retry", func(t *testing.T) {
		// Test that 500 Internal Server Error is NOT retried
		mockTransport := &mockRoundTripper{
			responses: []mockResponse{
				{resp: &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("server error"))}, err: nil},
			},
		}

		u, _ := url.Parse("http://example.com")
		mt := &multiTransport{
			rt:      mockTransport,
			targets: []*url.URL{u},
			keys:    []string{"test-key"},
		}

		req, _ := http.NewRequest("POST", "http://example.com", nil)
		resp, err := mt.RoundTrip(req)
		if err == nil && resp != nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.NotNil(t, resp, "Response should not be nil")
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Equal(t, 1, mockTransport.callCount, "Should have made only 1 attempt (no retry)")
	})

	t.Run("multiple_targets_with_status_retry", func(t *testing.T) {
		// Test retry behavior with multiple targets
		// Create a transport that routes to different mock transports based on URL
		routingTransport := &mockRoundTripper{}
		routingTransport.responses = []mockResponse{
			// First call to main endpoint (502)
			{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("bad gateway"))}, err: nil},
			// Second call to main endpoint (200)
			{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("success"))}, err: nil},
			// Call to additional endpoint (200)
			{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("additional success"))}, err: nil},
		}

		u1, _ := url.Parse("http://main.example.com")
		u2, _ := url.Parse("http://additional.example.com")
		mt := &multiTransport{
			rt:      routingTransport,
			targets: []*url.URL{u1, u2},
			keys:    []string{"main-key", "additional-key"},
		}

		// For multiple targets, we need a body since it gets read multiple times
		req, _ := http.NewRequest("POST", "http://example.com", io.NopCloser(strings.NewReader("test")))
		resp, err := mt.RoundTrip(req)
		if err == nil && resp != nil {
			defer resp.Body.Close()
		}

		assert.NoError(t, err)
		assert.NotNil(t, resp, "Response should not be nil")
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return main endpoint's successful response")
		assert.Equal(t, 3, routingTransport.callCount, "Should retry main endpoint once and call additional endpoint once")
	})
}

// bodyConsumingTransport is a test transport that actually reads request bodies
type bodyConsumingTransport struct {
	callCount  int
	bodiesRead []string
}

func (t *bodyConsumingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Actually read the body like a real HTTP transport would
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		t.bodiesRead = append(t.bodiesRead, string(bodyBytes))
	} else {
		t.bodiesRead = append(t.bodiesRead, "")
	}

	t.callCount++

	if t.callCount == 1 {
		// First attempt fails with 502
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
		}, nil
	}

	// Second attempt succeeds
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("success")),
	}, nil
}

func TestRetryWithActualBodyConsumption(t *testing.T) {
	// This test verifies that request bodies are properly handled in retries
	// by using a mock that actually consumes the request body like a real HTTP transport would

	transport := &bodyConsumingTransport{}

	u, _ := url.Parse("http://example.com")
	mt := &multiTransport{
		rt:      transport,
		targets: []*url.URL{u},
		keys:    []string{"test-key"},
	}

	// Create request with actual body content
	requestBody := "test-profile-data"
	req, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(requestBody))

	resp, err := mt.RoundTrip(req)
	if err == nil && resp != nil {
		defer resp.Body.Close()
	}

	// Verify the retry worked
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, transport.callCount, "Should have made 2 attempts")

	// Verify that both attempts received the same body content
	assert.Equal(t, 2, len(transport.bodiesRead), "Should have read 2 bodies")
	assert.Equal(t, requestBody, transport.bodiesRead[0], "First attempt should have received full body")
	assert.Equal(t, requestBody, transport.bodiesRead[1], "Second attempt should have received full body")

	// Verify both bodies are identical (proving the body was properly recreated for retry)
	assert.Equal(t, transport.bodiesRead[0], transport.bodiesRead[1],
		"Both attempts should have received identical body content")
}
