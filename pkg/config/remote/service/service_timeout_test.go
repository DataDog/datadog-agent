// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// hangingAPI simulates a degraded RC backend (e.g. rc-delivery behind rc-edge
// returning DeadlineExceeded) by blocking all Fetch and FetchOrgStatus calls
// until the context is cancelled or the test ends.
//
// This reproduces the production scenario from incident 52759: during a rolling
// update, freshly-started agents call refresh() which issues Fetch() with no
// context timeout. When the backend is degraded, the goroutine blocks
// indefinitely, preventing the agent from initializing and passing liveness
// probes.
type hangingAPI struct {
	fetchCalls     atomic.Int64
	orgStatusCalls atomic.Int64
	stopCh         chan struct{}
}

func newHangingAPI() *hangingAPI {
	return &hangingAPI{stopCh: make(chan struct{})}
}

func (h *hangingAPI) Fetch(ctx context.Context, _ *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	h.fetchCalls.Add(1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.stopCh:
		return nil, context.Canceled
	}
}

func (h *hangingAPI) FetchOrgData(ctx context.Context) (*pbgo.OrgDataResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.stopCh:
		return nil, context.Canceled
	}
}

func (h *hangingAPI) FetchOrgStatus(ctx context.Context) (*pbgo.OrgStatusResponse, error) {
	h.orgStatusCalls.Add(1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.stopCh:
		return nil, context.Canceled
	}
}

func (h *hangingAPI) UpdatePARJWT(_ string) {}
func (h *hangingAPI) UpdateAPIKey(_ string) {}

func (h *hangingAPI) stop() { close(h.stopCh) }

// newTestServiceWithHangingAPI creates a fresh CoreAgentService (simulating a
// post-rolling-update startup) wired to a hanging backend. The fetchTimeout
// parameter controls whether the fix is applied (short timeout) or not (very
// large timeout to simulate the pre-fix behavior).
func newTestServiceWithHangingAPI(t *testing.T, fetchTimeout time.Duration) (*CoreAgentService, *hangingAPI) {
	t.Helper()
	hangingBackend := newHangingAPI()
	uptaneClient := &mockCoreAgentUptane{}
	clk := clock.NewMock()

	opts := []Option{
		WithFetchTimeout(fetchTimeout),
	}
	service := newTestService(t, &mockAPI{}, uptaneClient, clk, opts...)
	service.api = hangingBackend

	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)
	uptaneClient.On("Update").Return(nil)
	uptaneClient.On("UnsafeTargetsMeta").Return([]byte(`{}`), nil)
	uptaneClient.On("TimestampExpires").Return(time.Now().Add(time.Hour), nil)

	return service, hangingBackend
}

// TestStartupRefreshBlocksOnHangingBackend reproduces the root cause of
// incident 52759. When a fresh service calls refresh() and the backend hangs,
// refresh() blocks indefinitely because it uses context.Background() with no
// timeout. This prevents the poll loop goroutine from advancing, which in
// production means the service cannot serve ClientGetConfigs and contributes
// to the liveness probe failure cascade.
//
// The test uses a very large fetchTimeout (simulating pre-fix behavior) and
// verifies that refresh() does NOT return within a reasonable window.
func TestStartupRefreshBlocksOnHangingBackend(t *testing.T) {
	service, hangingBackend := newTestServiceWithHangingAPI(t, 10*time.Minute)
	defer hangingBackend.stop()

	done := make(chan error, 1)
	go func() {
		done <- service.refresh()
	}()

	select {
	case <-done:
		t.Fatal("refresh() returned when it should have blocked indefinitely with a hanging backend and no effective timeout")
	case <-time.After(3 * time.Second):
		// Expected: refresh() is still blocked after 3s because the backend
		// hangs and there is no short timeout to bail out.
	}

	require.Greater(t, hangingBackend.fetchCalls.Load(), int64(0),
		"Fetch should have been called at least once")
}

// TestStartupRefreshTimesOutOnHangingBackend proves the fix works: with a
// short fetchTimeout, refresh() returns an error within the timeout window
// even when the backend hangs. This allows the poll loop to advance, apply
// backoff, and keep the service responsive during agent startup.
func TestStartupRefreshTimesOutOnHangingBackend(t *testing.T) {
	fetchTimeout := 500 * time.Millisecond
	service, hangingBackend := newTestServiceWithHangingAPI(t, fetchTimeout)
	defer hangingBackend.stop()

	done := make(chan error, 1)
	go func() {
		done <- service.refresh()
	}()

	select {
	case err := <-done:
		require.Error(t, err, "refresh() should return an error when the backend times out")
		assert.Contains(t, err.Error(), "context deadline exceeded",
			"error should indicate the fetch was cancelled by the timeout")
	case <-time.After(5 * time.Second):
		t.Fatal("refresh() did not return within 5s — timeout fix is not working")
	}
}

// TestOrgStatusPollerTimesOutOnHangingBackend proves the fix applies to the
// orgStatusPoller path as well. During startup, orgStatusPoller fires
// immediately (timer initialized to 0) and can also hang indefinitely on a
// degraded backend. With the fix, it times out and returns.
func TestOrgStatusPollerTimesOutOnHangingBackend(t *testing.T) {
	fetchTimeout := 500 * time.Millisecond
	service, hangingBackend := newTestServiceWithHangingAPI(t, fetchTimeout)
	defer hangingBackend.stop()

	done := make(chan struct{}, 1)
	go func() {
		service.orgStatusPoller.poll(service.api, service.rcType)
		done <- struct{}{}
	}()

	select {
	case <-done:
		// poll() returned — the timeout worked
	case <-time.After(5 * time.Second):
		t.Fatal("orgStatusPoller.poll() did not return within 5s — timeout fix is not working")
	}

	require.Greater(t, hangingBackend.orgStatusCalls.Load(), int64(0),
		"FetchOrgStatus should have been called")
}

// TestServiceResponsiveAfterTimeoutDuringStartup proves that after the first
// refresh() times out on a hanging backend, the service is still responsive.
// In the pre-fix world, a hanging refresh() would block the poll loop
// goroutine, making it unable to service bypass requests from
// ClientGetConfigs. With the fix, the poll loop advances past the failed
// refresh, applies backoff, and remains responsive.
func TestServiceResponsiveAfterTimeoutDuringStartup(t *testing.T) {
	fetchTimeout := 500 * time.Millisecond
	service, hangingBackend := newTestServiceWithHangingAPI(t, fetchTimeout)
	defer hangingBackend.stop()

	// Call refresh() directly to simulate the first startup call.
	// With the timeout, this should return quickly.
	err := service.refresh()
	require.Error(t, err, "first refresh should fail due to timeout")

	// Verify backoff was applied
	service.mu.Lock()
	backoffCount := service.mu.backoffErrorCount
	lastErr := service.mu.lastUpdateErr
	service.mu.Unlock()

	assert.Greater(t, backoffCount, 0,
		"backoff error count should be incremented after a failed refresh")
	assert.NotNil(t, lastErr,
		"lastUpdateErr should be set after a failed refresh")

	// Verify the service can still calculate a refresh interval (not wedged)
	interval := service.calculateRefreshInterval()
	assert.Greater(t, interval, time.Duration(0),
		"refresh interval should be positive after a failed refresh")
	assert.Greater(t, interval, service.mu.defaultRefreshInterval,
		"refresh interval should include backoff time after error")
}
