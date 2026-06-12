// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubTimeoutError is a net.Error that reports a timeout, used to exercise the
// timeout-handling branch of get() deterministically without real network I/O.
type stubTimeoutError struct{}

func (stubTimeoutError) Error() string   { return "simulated i/o timeout" }
func (stubTimeoutError) Timeout() bool   { return true }
func (stubTimeoutError) Temporary() bool { return true }

// stubRoundTripper lets a test swap the client's transport for one that always
// fails a given way.
type stubRoundTripper struct {
	err error
}

func (rt stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, rt.err
}

// TestGetReturnsTimeoutErrorWithoutPanicking locks the contract that a network
// timeout surfaces as a regular (retryable) error rather than a panic. Callers
// run get() inside polling loops (e.g. EventuallyWithTf); a panic there fails
// the whole test on a single transient blip instead of letting the next poll
// iteration retry.
func TestGetReturnsTimeoutErrorWithoutPanicking(t *testing.T) {
	client := NewClient("http://fakeintake.invalid",
		WithGetBackoffRetries(1),
		WithGetBackoffDelay(time.Millisecond),
	)
	client.httpClient = &http.Client{
		Transport: stubRoundTripper{err: &net.OpError{Op: "dial", Net: "tcp", Err: stubTimeoutError{}}},
	}

	var (
		body []byte
		err  error
	)
	require.NotPanics(t, func() {
		body, err = client.get("fakeintake/routestats")
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "fakeintake call timed out")
	require.Nil(t, body)
}

// TestGetReturnsNonTimeoutNetworkErrorWithoutPanicking ensures non-timeout
// network failures are also returned as errors (not panics), matching the
// timeout path.
func TestGetReturnsNonTimeoutNetworkErrorWithoutPanicking(t *testing.T) {
	client := NewClient("http://fakeintake.invalid",
		WithGetBackoffRetries(1),
		WithGetBackoffDelay(time.Millisecond),
	)
	client.httpClient = &http.Client{
		Transport: stubRoundTripper{err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.AddrError{Err: "connection refused"}}},
	}

	var err error
	require.NotPanics(t, func() {
		_, err = client.get("fakeintake/routestats")
	})
	require.Error(t, err)
	// A non-timeout failure must not be mislabeled as a timeout: this guards the
	// timeout/non-timeout discrimination in get().
	require.NotContains(t, err.Error(), "fakeintake call timed out")
}

// TestNewClientHTTPDialTimeout verifies the HTTP dial timeout is wired: a sane
// default is applied and WithHTTPDialTimeout overrides it.
func TestNewClientHTTPDialTimeout(t *testing.T) {
	def := NewClient("http://fakeintake.invalid")
	require.NotNil(t, def.httpClient)
	require.Equal(t, defaultHTTPDialTimeout, def.httpDialTimeout)

	custom := NewClient("http://fakeintake.invalid", WithHTTPDialTimeout(2*time.Second))
	require.NotNil(t, custom.httpClient)
	require.Equal(t, 2*time.Second, custom.httpDialTimeout)
}
