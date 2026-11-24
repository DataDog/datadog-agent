// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package remoteimpl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

type retryRoundTripper func(*http.Request) (*http.Response, error)

func (f retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type retryBody struct {
	io.Reader
	closed bool
}

func (b *retryBody) Close() error {
	b.closed = true
	return nil
}

type retryReadError struct{}

func (retryReadError) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func newRetryTestLoader(t *testing.T) *remoteIssueLoader {
	t.Helper()
	return newRemoteIssueLoader(Requires{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{"api_key": "api-key"}),
		Log:    logmock.New(t),
	}, flavor.DefaultAgent, &remoteTestIdentity{deploymentID: "retry-daemonset"})
}

func TestRemoteIssueLoaderRetriesTransientFetch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
		reader io.Reader
	}{
		{name: "connection refused", err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		{name: "temporary DNS", err: &net.DNSError{Err: "temporary lookup failure", Name: "api.example.com", IsTemporary: true}},
		{name: "interrupted body", status: http.StatusOK, reader: retryReadError{}},
		{name: "request timeout", status: http.StatusRequestTimeout},
		{name: "rate limited", status: http.StatusTooManyRequests},
		{name: "server error", status: http.StatusInternalServerError},
		{name: "unavailable", status: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loader := newRetryTestLoader(t)
			payload := snapshotResponse(t, "retry-daemonset", []remoteIssue{{IssueID: "restored-issue"}})
			synctest.Test(t, func(t *testing.T) {
				attempts, identityCalls := 0, 0
				var previous *retryBody
				loader.resourceID = func(context.Context) string {
					identityCalls++
					return "retry-daemonset"
				}
				loader.httpClient.Transport = retryRoundTripper(func(*http.Request) (*http.Response, error) {
					if previous != nil {
						assert.True(t, previous.closed, "close the previous response before retrying")
					}
					attempts++
					if attempts == 1 {
						if tc.err != nil {
							return nil, tc.err
						}
						reader := tc.reader
						if reader == nil {
							reader = strings.NewReader("temporarily unavailable")
						}
						previous = &retryBody{Reader: reader}
						return &http.Response{StatusCode: tc.status, Body: previous}, nil
					}
					previous = &retryBody{Reader: strings.NewReader(string(payload))}
					return &http.Response{StatusCode: http.StatusOK, Body: previous}, nil
				})

				start := time.Now()
				issues, err := loader.Load(context.Background())
				require.NoError(t, err)
				require.Len(t, issues, 1)
				assert.Equal(t, "restored-issue", issues[0].IssueID)
				assert.Equal(t, 2, attempts)
				assert.Equal(t, 1, identityCalls, "resolve identity once for the whole retry operation")
				assert.Positive(t, time.Since(start), "back off before a retry")
				assert.True(t, previous.closed)
			})
		})
	}
}

func TestRemoteIssueLoaderDoesNotRetryEmptySnapshot(t *testing.T) {
	loader := newRetryTestLoader(t)
	payload := snapshotResponse(t, "retry-daemonset", []remoteIssue{})
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		body := &retryBody{Reader: strings.NewReader(string(payload))}
		loader.httpClient.Transport = retryRoundTripper(func(*http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		})
		start := time.Now()
		issues, err := loader.Load(context.Background())
		require.NoError(t, err)
		assert.Empty(t, issues)
		assert.Equal(t, 1, attempts)
		assert.Zero(t, time.Since(start))
		assert.True(t, body.closed)
	})
}

func TestRemoteIssueLoaderDoesNotRetryPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		err        error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "forbidden", status: http.StatusForbidden},
		{name: "not found", status: http.StatusNotFound},
		{name: "redirect", status: http.StatusFound},
		{name: "certificate verification", err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}},
		{name: "malformed snapshot", status: http.StatusOK, body: `{"data":`},
		{name: "oversized snapshot", status: http.StatusOK, body: strings.Repeat("x", remoteIssuesMaxResponse+1)},
		{name: "wrong identity", status: http.StatusOK, body: `{"data":{"id":"other","type":"agent_health_issues","attributes":{"issues":[]}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loader := newRetryTestLoader(t)
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				body := &retryBody{Reader: strings.NewReader(tc.body)}
				loader.httpClient.Transport = retryRoundTripper(func(*http.Request) (*http.Response, error) {
					attempts++
					if tc.err != nil {
						return nil, tc.err
					}
					return &http.Response{StatusCode: tc.status, Body: body}, nil
				})
				start := time.Now()
				issues, err := loader.Load(context.Background())
				assert.Nil(t, issues)
				require.Error(t, err)
				assert.Equal(t, 1, attempts)
				assert.Zero(t, time.Since(start), "permanent errors must not enter backoff")
				if tc.err == nil {
					assert.True(t, body.closed)
				}
			})
		})
	}
}

func TestRemoteIssueLoaderRetryBudgetIncludesInFlightRequest(t *testing.T) {
	for _, parentTimeout := range []time.Duration{time.Minute, 5 * time.Second} {
		t.Run(parentTimeout.String(), func(t *testing.T) {
			loader := newRetryTestLoader(t)
			synctest.Test(t, func(t *testing.T) {
				const identityDelay = 2 * time.Second
				identityCalls, attempts := 0, 0
				loader.resourceID = func(context.Context) string {
					identityCalls++
					time.Sleep(identityDelay)
					return "retry-daemonset"
				}
				body := &retryBody{Reader: strings.NewReader("unavailable")}
				loader.httpClient.Transport = retryRoundTripper(func(req *http.Request) (*http.Response, error) {
					attempts++
					if attempts == 1 {
						time.Sleep(time.Second)
						return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil
					}
					assert.True(t, body.closed)
					<-req.Context().Done()
					return nil, req.Context().Err()
				})
				ctx, cancel := context.WithTimeout(context.Background(), parentTimeout)
				defer cancel()
				start := time.Now()
				issues, err := loader.Load(ctx)
				assert.Nil(t, issues)
				require.ErrorIs(t, err, context.DeadlineExceeded)
				assert.Equal(t, min(parentTimeout, identityDelay+10*time.Second), time.Since(start), "identity and HTTP have separate budgets; retries share one HTTP deadline")
				assert.Equal(t, 2, attempts)
				assert.Equal(t, 1, identityCalls)
			})
		})
	}
}

func TestRemoteIssueLoaderCancelRetry(t *testing.T) {
	for _, phase := range []string{"backoff", "in-flight"} {
		t.Run(phase, func(t *testing.T) {
			loader := newRetryTestLoader(t)
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				started := make(chan struct{})
				attempts := 0
				body := &retryBody{Reader: strings.NewReader("unavailable")}
				loader.httpClient.Transport = retryRoundTripper(func(req *http.Request) (*http.Response, error) {
					attempts++
					if attempts == 1 {
						close(started)
					}
					if phase == "in-flight" {
						<-req.Context().Done()
						return nil, req.Context().Err()
					}
					return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil
				})
				done := make(chan error, 1)
				start := time.Now()
				go func() {
					issues, err := loader.Load(ctx)
					assert.Nil(t, issues)
					done <- err
				}()
				<-started
				synctest.Wait()
				cancel()
				require.ErrorIs(t, <-done, context.Canceled)
				assert.Equal(t, 1, attempts)
				assert.Zero(t, time.Since(start), "cancellation must interrupt the wait immediately")
				if phase == "backoff" {
					assert.True(t, body.closed)
				}
			})
		})
	}
}

func TestRemoteIssueLoaderRetryAfterCannotExtendBudget(t *testing.T) {
	loader := newRetryTestLoader(t)
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		body := &retryBody{Reader: strings.NewReader("unavailable")}
		loader.httpClient.Transport = retryRoundTripper(func(*http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     http.Header{"Retry-After": []string{"60"}},
				Body:       body,
			}, nil
		})
		start := time.Now()
		issues, err := loader.Load(context.Background())
		assert.Nil(t, issues)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, 10*time.Second, time.Since(start))
		assert.Equal(t, 1, attempts)
		assert.True(t, body.closed)
	})
}
