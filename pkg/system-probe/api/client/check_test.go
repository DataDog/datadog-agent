// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

func startTestServer(t *testing.T, handler http.Handler) (string, *httptest.Server) {
	t.Helper()

	socketPath := testutil.SystemProbeSocketPath(t, "client")
	server, err := testutil.NewSystemProbeTestServer(handler, socketPath)
	require.NoError(t, err)
	require.NotNil(t, server)
	server.Start()
	t.Cleanup(server.Close)

	return socketPath, server
}

func resetStartupChecker() {
	checker := getStartChecker()
	checker.mutex.Lock()
	defer checker.mutex.Unlock()
	checker.startTime = time.Now()
	checker.started = false
	checker.inFlight = nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func successfulCheckResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
	}
}

func TestConstructURL(t *testing.T) {
	u := constructURL("", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/asdf?a=b", u)

	u = constructURL("zzzz", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/zzzz/asdf?a=b", u)

	u = constructURL("zzzz", "asdf")
	assert.Equal(t, "http://sysprobe/zzzz/asdf", u)
}

type expectedTelemetryValues struct {
	totalRequests      float64
	failedRequests     float64
	failedResponses    float64
	responseErrors     float64
	malformedResponses float64
}

type testData struct {
	Str string
	Num int
}

func validateTelemetry(t *testing.T, module string, expected expectedTelemetryValues) {
	assert.Equal(t, expected.totalRequests, checkTelemetry.totalRequests.WithValues(module).Get(), "mismatched totalRequests counter value")
	assert.Equal(t, expected.failedRequests, checkTelemetry.failedRequests.WithValues(module).Get(), "mismatched failedRequest counter value")
	assert.Equal(t, expected.failedResponses, checkTelemetry.failedResponses.WithValues(module).Get(), "mismatched failedResponses counter value")
	assert.Equal(t, expected.responseErrors, checkTelemetry.responseErrors.WithValues(module).Get(), "mismatched responseErrors counter value")
	assert.Equal(t, expected.malformedResponses, checkTelemetry.malformedResponses.WithValues(module).Get(), "mismatched malformedResponses counter value")
}

type requestData struct {
	Pids []int32
}

func TestGetCheck(t *testing.T) {
	t.Cleanup(resetStartupChecker)

	socketPath, server := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test/check" {
			assert.Equal(t, http.MethodGet, r.Method, "check endpoint should use GET")
			_, _ = w.Write([]byte(`{"Str": "asdf", "Num": 42}`))
		} else if r.URL.Path == "/test/services" {
			assert.Equal(t, http.MethodPost, r.Method, "services endpoint should use POST")
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var reqData requestData
			err = json.Unmarshal(body, &reqData)
			require.NoError(t, err)
			assert.Equal(t, []int32{1, 2, 3}, reqData.Pids)

			_, _ = w.Write([]byte(`{"Str": "with_body", "Num": 99}`))
		} else if r.URL.Path == "/malformed/check" {
			//this should fail in json.Unmarshal
			_, _ = w.Write([]byte("1"))
		} else if r.URL.Path == "/debug/stats" {
			_, _ = w.Write([]byte(`{}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	client := GetCheckClient(WithSocketPath(socketPath))

	//test happy flow
	resp, err := GetCheck[testData](client, "test")
	require.NoError(t, err)
	assert.Equal(t, "asdf", resp.Str)
	assert.Equal(t, 42, resp.Num)
	validateTelemetry(t, "test", expectedTelemetryValues{1, 0, 0, 0, 0})

	//test Post with request body
	requestBody := requestData{Pids: []int32{1, 2, 3}}
	resp, err = Post[testData](client, "/services", requestBody, "test")
	require.NoError(t, err)
	assert.Equal(t, "with_body", resp.Str)
	assert.Equal(t, 99, resp.Num)
	validateTelemetry(t, "test", expectedTelemetryValues{2, 0, 0, 0, 0})

	//test responseError counter
	resp, err = GetCheck[testData](client, "foo")
	require.Error(t, err)
	validateTelemetry(t, "foo", expectedTelemetryValues{1, 0, 0, 1, 0})

	//test malformedResponses counter
	resp, err = GetCheck[testData](client, "malformed")
	require.Error(t, err)
	validateTelemetry(t, "malformed", expectedTelemetryValues{1, 0, 0, 0, 1})

	//test failedRequests counter
	server.Close()
	resp, err = GetCheck[testData](client, "test")
	require.Error(t, err)
	validateTelemetry(t, "test", expectedTelemetryValues{3, 1, 0, 0, 0})
}

func TestGetCheckStartup(t *testing.T) {
	t.Cleanup(resetStartupChecker)

	failRequest := true
	socketPath, _ := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failRequest {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if r.URL.Path == "/debug/stats" {
			_, _ = w.Write([]byte(`{}`))
		} else if r.URL.Path == "/test/check" {
			_, _ = w.Write([]byte(`{"Num": 42}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	client := GetCheckClient(WithSocketPath(socketPath))

	_, err := GetCheck[testData](client, "test")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotStartedYet)

	// Test after grace period
	client.startupChecker.mutex.Lock()
	client.startupChecker.startTime = time.Now().Add(-6 * time.Minute)
	client.startupChecker.mutex.Unlock()

	// The error should not be ErrNotStartedYet since we're past the grace period
	_, err = GetCheck[testData](client, "test")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotStartedYet)

	failRequest = false

	// Test successful check after startup
	resp, err := GetCheck[testData](client, "test")
	require.NoError(t, err)
	assert.Equal(t, 42, resp.Num)
}

func TestStartCheckerCanceledWaiterDoesNotWaitForBlockedProbe(t *testing.T) {
	firstStarted := make(chan struct{})
	var once sync.Once
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		once.Do(func() { close(firstStarted) })
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	checker := &startChecker{
		startTime:      time.Now(),
		startupTimeout: time.Minute,
	}

	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	ownerDone := make(chan error)
	go func() {
		ownerDone <- checker.ensureStarted(ownerCtx, httpClient)
	}()
	<-firstStarted

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	cancelWaiter()
	require.ErrorIs(t, checker.ensureStarted(waiterCtx, httpClient), context.Canceled)

	select {
	case err := <-ownerDone:
		require.Failf(t, "probe owner returned before cancellation", "error: %v", err)
	default:
	}
	cancelOwner()
	require.ErrorIs(t, <-ownerDone, context.Canceled)
}

func TestStartCheckerWaiterRetriesAfterProbeOwnerCancellation(t *testing.T) {
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		if call == 1 {
			close(firstStarted)
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		close(secondStarted)
		return successfulCheckResponse(), nil
	})}
	checker := &startChecker{
		startTime:      time.Now(),
		startupTimeout: time.Minute,
	}

	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	ownerDone := make(chan error)
	go func() {
		ownerDone <- checker.ensureStarted(ownerCtx, httpClient)
	}()
	<-firstStarted

	waiterDone := make(chan error)
	go func() {
		waiterDone <- checker.ensureStarted(context.Background(), httpClient)
	}()
	cancelOwner()

	require.ErrorIs(t, <-ownerDone, context.Canceled)
	<-secondStarted
	require.NoError(t, <-waiterDone)
	mu.Lock()
	assert.Equal(t, 2, calls)
	mu.Unlock()
}

func TestGetCheckWithContextCancelsModuleRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(requestStarted)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	checker := &startChecker{started: true}
	client := NewCheckClient(httpClient, httpClient)
	client.startupChecker = checker

	ctx, cancel := context.WithCancel(context.Background())
	requestDone := make(chan error)
	go func() {
		_, err := GetCheckWithContext[testData](ctx, client, "context-cancel")
		requestDone <- err
	}()
	<-requestStarted
	cancel()

	require.Error(t, <-requestDone)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}
