// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRoundTripper struct {
	resp *http.Response
	err  error
}

func (s stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return s.resp, s.err
}

// timeoutError implements net.Error reporting a timeout.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func TestBackoffSignalTransport(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		err        error
		wantSignal bool
	}{
		{name: "200 ok", statusCode: 200},
		{name: "408 request timeout", statusCode: http.StatusRequestTimeout, wantSignal: true},
		{name: "429 too many requests", statusCode: http.StatusTooManyRequests, wantSignal: true},
		{name: "500 not a backoff signal", statusCode: 500},
		{name: "503 service unavailable", statusCode: http.StatusServiceUnavailable, wantSignal: true},
		{name: "net timeout", err: timeoutError{}, wantSignal: true},
		{name: "deadline exceeded", err: context.DeadlineExceeded, wantSignal: true},
		{name: "context canceled (shutdown)", err: context.Canceled},
		{name: "other error", err: errors.New("connection refused")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stubResp *http.Response
			if tc.err == nil {
				stubResp = &http.Response{StatusCode: tc.statusCode, Body: http.NoBody}
			}
			signals := 0
			rt := &backoffSignalTransport{
				base:      stubRoundTripper{resp: stubResp, err: tc.err},
				onBackoff: func() { signals++ },
			}

			req, err := http.NewRequest(http.MethodPost, "http://example.com", nil)
			require.NoError(t, err)
			resp, _ := rt.RoundTrip(req)
			if resp != nil {
				_ = resp.Body.Close()
			}

			if tc.wantSignal {
				assert.Equal(t, 1, signals)
			} else {
				assert.Equal(t, 0, signals)
			}
		})
	}
}

func TestSignalBackoffCoalesces(t *testing.T) {
	sc := &SharedConnection{backoffCh: make(chan struct{}, 1)}

	// Multiple signals collapse onto the size-1 channel without blocking.
	sc.signalBackoff()
	sc.signalBackoff()
	sc.signalBackoff()

	select {
	case <-sc.backoffCh:
	default:
		t.Fatal("expected a pending backoff signal")
	}
	select {
	case <-sc.backoffCh:
		t.Fatal("expected only one coalesced signal")
	default:
	}
}
