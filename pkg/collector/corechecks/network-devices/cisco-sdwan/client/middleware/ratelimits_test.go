// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package middleware

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// stubTransport returns an empty 200 response and records the requests it receives
type stubTransport struct {
	requests []*http.Request
}

func (rt *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.requests = append(rt.requests, req)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

func newTestRequest(t *testing.T, ctx context.Context) *http.Request {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://sdwan.test/test", nil)
	require.NoError(t, err)
	return req
}

func TestRateLimitedTransportPacesRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		next := &stubTransport{}
		transport := NewRateLimitedTransport(next, rate.NewLimiter(10, 1), time.Second)
		start := time.Now()

		var sent []time.Duration
		for range 3 {
			resp, err := transport.RoundTrip(newTestRequest(t, context.Background()))
			require.NoError(t, err)
			resp.Body.Close()
			sent = append(sent, time.Since(start))
		}

		require.Equal(t, []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond}, sent)
		require.Len(t, next.requests, 3)
	})
}

func TestRateLimitedTransportMaxWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		next := &stubTransport{}
		transport := NewRateLimitedTransport(next, rate.NewLimiter(0.001, 1), 100*time.Millisecond)
		start := time.Now()

		_, err := transport.RoundTrip(newTestRequest(t, context.Background()))
		require.NoError(t, err)

		// The next token is ~1000s away, the transport fails without waiting for it
		_, err = transport.RoundTrip(newTestRequest(t, context.Background()))
		require.ErrorIs(t, err, ErrRateLimitTimeout)
		require.Zero(t, time.Since(start))
		require.Len(t, next.requests, 1)
	})
}

func TestRateLimitedTransportCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		next := &stubTransport{}
		transport := NewRateLimitedTransport(next, rate.NewLimiter(0.01, 1), time.Hour)
		start := time.Now()

		_, err := transport.RoundTrip(newTestRequest(t, context.Background()))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)

		_, err = transport.RoundTrip(newTestRequest(t, ctx))
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, ErrRateLimitTimeout)
		require.Equal(t, 100*time.Millisecond, time.Since(start))
		require.Len(t, next.requests, 1)
	})
}

func TestNewRateLimitedTransportDefaultsToDefaultTransport(t *testing.T) {
	transport := NewRateLimitedTransport(nil, rate.NewLimiter(rate.Inf, 1), time.Second)
	require.Equal(t, http.DefaultTransport, transport.next)
}
