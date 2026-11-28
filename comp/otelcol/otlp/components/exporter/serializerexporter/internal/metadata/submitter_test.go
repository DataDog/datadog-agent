// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"golang.org/x/time/rate"
)

type receivedRequest struct {
	method, path, apiKey, appKey, body string
}

// startServer starts a server that replies with statuses in order, then with 200 OK.
func startServer(t *testing.T, statuses ...int) (string, <-chan receivedRequest) {
	requests := make(chan receivedRequest, 10)
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- receivedRequest{r.Method, r.URL.Path, r.Header.Get("DD-API-KEY"), r.Header.Get("DD-APPLICATION-KEY"), string(body)}
		mu.Lock()
		defer mu.Unlock()
		if len(statuses) > 0 {
			w.WriteHeader(statuses[0])
			statuses = statuses[1:]
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, requests
}

// newSubmitter creates a [Submitter] without a rate limit or delay between retries.
func newSubmitter(t *testing.T, endpoint string) *Submitter {
	s, err := NewSubmitter(componenttest.NewNopTelemetrySettings(), confighttp.NewDefaultClientConfig(), endpoint, "api-key", "app-key")
	require.NoError(t, err)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	s.backOff = &backoff.ZeroBackOff{}
	return s
}

func startSubmitter(t *testing.T, endpoint string) *Submitter {
	s := newSubmitter(t, endpoint)
	require.NoError(t, s.Start(context.Background(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, s.Shutdown(context.Background())) })
	return s
}

func receive(t *testing.T, requests <-chan receivedRequest) receivedRequest {
	select {
	case r := <-requests:
		return r
	case <-time.After(10 * time.Second):
		require.FailNow(t, "timed out waiting for request")
		return receivedRequest{}
	}
}

func TestSubmit(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		wantBody string
	}{
		{"unit", "byte", `{"description":"Bytes read.","unit":"byte"}`},
		{"rate", "byte/second", `{"description":"Bytes read.","unit":"byte","per_unit":"second"}`},
		{"no unit", "", `{"description":"Bytes read."}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, requests := startServer(t)
			s := startSubmitter(t, endpoint)

			s.Submit(Metadata{Name: "test.metric", Description: "Bytes read.", Unit: tt.unit})

			assert.Equal(t, receivedRequest{
				method: http.MethodPut,
				path:   "/api/v1/metrics/test.metric",
				apiKey: "api-key",
				appKey: "app-key",
				body:   tt.wantBody,
			}, receive(t, requests))
		})
	}
}

func TestSubmitSkipsSubmittedMetadata(t *testing.T) {
	endpoint, requests := startServer(t)
	s := startSubmitter(t, endpoint)

	for _, description := range []string{"Foo", "Foo", "Bar", "Foo"} {
		s.Submit(Metadata{Name: "test.metric", Description: description})
	}

	assert.JSONEq(t, `{"description":"Foo"}`, receive(t, requests).body)
	assert.JSONEq(t, `{"description":"Bar"}`, receive(t, requests).body)
	assert.JSONEq(t, `{"description":"Foo"}`, receive(t, requests).body)
}

func TestSubmitRetries(t *testing.T) {
	tests := []struct {
		status    int
		wantRetry bool
	}{
		{http.StatusInternalServerError, true},
		{http.StatusTooManyRequests, true},
		{http.StatusNotFound, false},
		{http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			endpoint, requests := startServer(t, tt.status)
			s := startSubmitter(t, endpoint)

			s.Submit(Metadata{Name: "first.metric", Description: "First."})
			s.Submit(Metadata{Name: "second.metric", Description: "Second."})

			assert.Equal(t, "/api/v1/metrics/first.metric", receive(t, requests).path)
			wantNext := "/api/v1/metrics/second.metric"
			if tt.wantRetry {
				wantNext = "/api/v1/metrics/first.metric"
			}
			assert.Equal(t, wantNext, receive(t, requests).path)
		})
	}
}

func TestSubmitResubmitsDroppedMetadata(t *testing.T) {
	s := newSubmitter(t, "http://localhost")
	for i := range queueSize {
		s.Submit(Metadata{Name: fmt.Sprintf("metric.%d", i), Description: "A metric."})
	}
	dropped := Metadata{Name: "dropped.metric", Description: "Dropped."}
	s.Submit(dropped)
	require.Len(t, s.queue, queueSize)

	for range queueSize {
		<-s.queue
	}
	s.Submit(dropped)
	require.Len(t, s.queue, 1)
	assert.Equal(t, dropped, <-s.queue)
}

func TestSubmitAfterFailure(t *testing.T) {
	tests := []struct {
		status          int
		wantResubmitted bool
	}{
		{http.StatusInternalServerError, true},
		{http.StatusNotFound, false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			endpoint, requests := startServer(t, tt.status)
			s := newSubmitter(t, endpoint)
			s.backOff = &backoff.StopBackOff{}
			require.NoError(t, s.Start(context.Background(), componenttest.NewNopHost()))
			t.Cleanup(func() { require.NoError(t, s.Shutdown(context.Background())) })

			first := Metadata{Name: "first.metric", Description: "First."}
			s.Submit(first)
			assert.Equal(t, "/api/v1/metrics/first.metric", receive(t, requests).path)
			require.Eventually(t, func() bool {
				s.mu.Lock()
				defer s.mu.Unlock()
				return len(s.pending) == 0
			}, 10*time.Second, time.Millisecond)

			s.Submit(first, Metadata{Name: "second.metric", Description: "Second."})

			wantNext := "/api/v1/metrics/second.metric"
			if tt.wantResubmitted {
				wantNext = "/api/v1/metrics/first.metric"
			}
			assert.Equal(t, wantNext, receive(t, requests).path)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSubmitRateLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := NewSubmitter(componenttest.NewNopTelemetrySettings(), confighttp.NewDefaultClientConfig(), "http://localhost", "api-key", "app-key")
		require.NoError(t, err)
		var requests atomic.Int64
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		})}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.run(ctx, client)
		}()

		// Check a tenth of the interval: the queue cannot hold twice rateLimitRequests.
		const wantRequests = rateLimitRequests / 10
		for i := range 2 * wantRequests {
			s.Submit(Metadata{Name: fmt.Sprintf("metric.%d", i), Description: "A metric."})
		}
		time.Sleep(rateLimitInterval/10 - time.Nanosecond)
		synctest.Wait()
		assert.EqualValues(t, wantRequests, requests.Load())

		cancel()
		<-done
	})
}
