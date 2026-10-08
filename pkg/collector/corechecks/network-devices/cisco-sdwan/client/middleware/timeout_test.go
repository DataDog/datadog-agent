// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package middleware

import (
	"context"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// blockingTransport blocks until the request context is done
type blockingTransport struct{}

func (blockingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestTimeoutTransportTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := NewTimeoutTransport(blockingTransport{}, 10*time.Second)
		start := time.Now()

		_, err := transport.RoundTrip(newTestRequest(t, context.Background()))
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, 10*time.Second, time.Since(start))
	})
}

func TestTimeoutTransportReleasesDeadlineOnBodyClose(t *testing.T) {
	next := &stubTransport{}
	transport := NewTimeoutTransport(next, time.Hour)
	parent := newTestRequest(t, context.Background())

	resp, err := transport.RoundTrip(parent)
	require.NoError(t, err)

	// The original request is not modified
	require.Len(t, next.requests, 1)
	sent := next.requests[0]
	require.NotSame(t, parent, sent)
	_, hasDeadline := parent.Context().Deadline()
	require.False(t, hasDeadline)
	_, hasDeadline = sent.Context().Deadline()
	require.True(t, hasDeadline)

	// The body can still be read after RoundTrip returns
	require.NoError(t, sent.Context().Err())
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))

	require.NoError(t, resp.Body.Close())
	require.ErrorIs(t, sent.Context().Err(), context.Canceled)
}

func TestNewTimeoutTransportDefaultsToDefaultTransport(t *testing.T) {
	transport := NewTimeoutTransport(nil, time.Second)
	require.Equal(t, http.DefaultTransport, transport.next)
}
