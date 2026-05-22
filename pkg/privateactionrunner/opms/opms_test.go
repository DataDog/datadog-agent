// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package opms

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/jsonapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
)

// newTestKey generates a throwaway ECDSA key for unit tests.
func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

// newTestClient builds a client wired to the given httptest server.
// It sets DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION so endpointURL uses plain HTTP.
func newTestClient(t *testing.T, srv *httptest.Server) *client {
	t.Helper()
	t.Setenv(app.InternalSkipTaskVerificationEnvVar, "true")
	return &client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		config: &config.Config{
			DDHost:             srv.URL, // "http://127.0.0.1:PORT"
			OpmsRequestTimeout: 5000,
			OrgId:              1,
			RunnerId:           "test-runner",
			PrivateKey:         newTestKey(t),
		},
		runnerStartedAt: time.Now().UTC(),
	}
}

// ---------- parseRetryAfterMs ----------

func TestParseRetryAfterMs(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		expected time.Duration
	}{
		{"nil headers", nil, 0},
		{"absent header", http.Header{}, 0},
		{"zero value", http.Header{retryAfterMsHeader: []string{"0"}}, 0},
		{"negative value", http.Header{retryAfterMsHeader: []string{"-100"}}, 0},
		{"non-numeric value", http.Header{retryAfterMsHeader: []string{"soon"}}, 0},
		{"500 ms", http.Header{retryAfterMsHeader: []string{"500"}}, 500 * time.Millisecond},
		{"1000 ms", http.Header{retryAfterMsHeader: []string{"1000"}}, time.Second},
		{"at clamp", http.Header{retryAfterMsHeader: []string{"120000"}}, maxRetryAfter},
		{"above clamp is capped", http.Header{retryAfterMsHeader: []string{"300000"}}, maxRetryAfter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseRetryAfterMs(tt.headers))
		})
	}
}

// ---------- DequeueTask ----------

func TestDequeueTask_RetryAfterMs_EmptyResponse(t *testing.T) {
	// Server returns no task body and a retry-after hint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(retryAfterMsHeader, "2000")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	task, retryAfter, err := c.DequeueTask(context.Background())

	require.NoError(t, err)
	assert.Nil(t, task, "empty body should produce a nil task")
	assert.Equal(t, 2000*time.Millisecond, retryAfter)
}

func TestDequeueTask_RetryAfterMs_ZeroMeansDefault(t *testing.T) {
	// Server sends the header but with value 0 — caller should use default interval.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(retryAfterMsHeader, "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, retryAfter, err := c.DequeueTask(context.Background())

	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), retryAfter, "zero value should signal 'use default'")
}

func TestDequeueTask_RetryAfterMs_AbsentHeader(t *testing.T) {
	// Server does not include the header — retryAfter should be 0.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, retryAfter, err := c.DequeueTask(context.Background())

	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), retryAfter)
}

// ---------- dequeue request body ----------

// TestDequeueTask_RequestBody walks the runner through a scripted sequence of
// server responses and asserts what each request body contains:
//   - runner_started_at is on every request and stable
//   - last_task_received_at is omitted until a non-empty dequeue succeeds, then
//     populated, and not updated by empty or error responses.
func TestDequeueTask_RequestBody(t *testing.T) {
	empty := func(w http.ResponseWriter) { w.WriteHeader(http.StatusOK) }
	fail := func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) }
	task := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"task-1"}}`))
	}

	steps := []struct {
		name                   string
		respond                func(w http.ResponseWriter)
		wantErr                bool
		wantLastTaskReceivedAt bool
	}{
		{"first call, no prior task", empty, false, false},
		{"error response does not record a timestamp", fail, true, false},
		{"empty response does not record a timestamp", empty, false, false},
		{"successful task: body sent before timestamp is recorded", task, false, false},
		{"after a successful task, body carries the timestamp", empty, false, true},
	}

	bodies := make(chan DequeueJSONRequest, len(steps))
	var i int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body DequeueJSONRequest
		require.NoError(t, jsonapi.Unmarshal(raw, &body))
		bodies <- body
		steps[i].respond(w)
		i++
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	runnerStartedAt := c.runnerStartedAt.Format(time.RFC3339)

	for _, s := range steps {
		_, _, err := c.DequeueTask(context.Background())
		if s.wantErr {
			require.Error(t, err, s.name)
		} else {
			require.NoError(t, err, s.name)
		}

		body := <-bodies
		assert.Equal(t, runnerStartedAt, body.RunnerStartedAt, "%s: runner_started_at", s.name)

		if s.wantLastTaskReceivedAt {
			require.NotEmpty(t, body.LastTaskReceivedAt, "%s: last_task_received_at must be set", s.name)
			_, err := time.Parse(time.RFC3339, body.LastTaskReceivedAt)
			require.NoError(t, err, "%s: last_task_received_at must be RFC3339", s.name)
		} else {
			assert.Empty(t, body.LastTaskReceivedAt, "%s: last_task_received_at must be empty", s.name)
		}
	}
}

// ---------- DequeueTask error handling ----------

func TestDequeueTask_RetryAfterMs_OnErrorResponse(t *testing.T) {
	// Server returns a 429 with a retry-after hint.
	// The error is surfaced, but the duration is still parsed from headers.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(retryAfterMsHeader, "5000")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, retryAfter, err := c.DequeueTask(context.Background())

	require.Error(t, err)
	assert.Equal(t, 5000*time.Millisecond, retryAfter, "retry-after should still be parsed on error response")
}

// ---------- HealthCheck ----------

func TestHealthCheck_RetryAfterMs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(retryAfterMsHeader, "1500")
		w.Header().Set(serverTimeHeader, "2025-01-01T00:00:00Z")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	data, err := c.HealthCheck(context.Background())

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Equal(t, 1500*time.Millisecond, data.RetryAfter)
	require.NotNil(t, data.ServerTime)
}

func TestHealthCheck_RetryAfterMs_ZeroMeansDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(retryAfterMsHeader, "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	data, err := c.HealthCheck(context.Background())

	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), data.RetryAfter)
}

func TestHealthCheck_RetryAfterMs_PopulatedOnError(t *testing.T) {
	// Even a failing health check should return the retry-after hint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(retryAfterMsHeader, "3000")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	data, err := c.HealthCheck(context.Background())

	require.Error(t, err)
	require.NotNil(t, data)
	assert.Equal(t, 3000*time.Millisecond, data.RetryAfter)
}
