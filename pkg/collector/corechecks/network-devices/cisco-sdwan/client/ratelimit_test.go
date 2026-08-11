// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newRateTestClient(t *testing.T, server *httptest.Server, opts ...ClientOptions) *Client {
	c, err := NewClient(serverURL(server), "testuser", "testpass", true, opts...)
	require.NoError(t, err)
	return c
}

func rateTestServer(t *testing.T) *httptest.Server {
	mux := setupCommonServerMux()
	mux.HandleFunc("/data", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// A non-positive rate (or no option at all) leaves the client unpaced.
func TestWithRequestRateDisabledByDefault(t *testing.T) {
	c, err := NewClient("host", "testuser", "testpass", true)
	require.NoError(t, err)
	require.Nil(t, c.limiter)

	c, err = NewClient("host", "testuser", "testpass", true, WithRequestRate(0, 5))
	require.NoError(t, err)
	require.Nil(t, c.limiter)

	c, err = NewClient("host", "testuser", "testpass", true, WithRequestRate(-1, 5))
	require.NoError(t, err)
	require.Nil(t, c.limiter)
}

// A burst below 1 is clamped so the limiter always admits requests.
func TestWithRequestRateClampsBurst(t *testing.T) {
	c, err := NewClient("host", "testuser", "testpass", true, WithRequestRate(10, 0))
	require.NoError(t, err)
	require.NotNil(t, c.limiter)
	require.Equal(t, 1, c.limiter.Burst())
}

// Requests are spread across time instead of bursting all at once.
func TestWithRequestRatePacesRequests(t *testing.T) {
	server := rateTestServer(t)

	// 20 req/s, burst 1: after the initial token each request waits ~50ms.
	c := newRateTestClient(t, server, WithRequestRate(20, 1))

	const n = 4
	start := time.Now()
	for i := 0; i < n; i++ {
		_, err := c.get("/data", nil)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	// (n-1) paced gaps of ~50ms = ~150ms minimum. Assert a conservative lower
	// bound; the limiter guarantees the minimum spacing but never less.
	minExpected := time.Duration(n-1)*50*time.Millisecond - 15*time.Millisecond
	require.GreaterOrEqual(t, elapsed, minExpected,
		"expected requests to be paced over at least %s, took %s", minExpected, elapsed)
}

// A cancelled context aborts a pacing wait promptly instead of blocking.
func TestWithContextCancelsPacing(t *testing.T) {
	server := rateTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	// Very slow rate: after the first (burst) token, the next request would wait
	// ~1000s unless the context aborts it.
	c := newRateTestClient(t, server, WithRequestRate(0.001, 1), WithContext(ctx))

	// First request consumes the burst token and returns promptly.
	_, err := c.get("/data", nil)
	require.NoError(t, err)

	cancel()

	start := time.Now()
	_, err = c.get("/data", nil)
	require.Error(t, err)
	require.Less(t, time.Since(start), time.Second, "cancelled pacing wait should return promptly")
}
