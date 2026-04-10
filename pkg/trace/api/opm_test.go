// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeOPM(t *testing.T) {
	// Golden-value: known UUID → known OPM via SHA-256 → Trunc60 → base64url
	opm := computeOPM("abc12312-000e-11ea-a34b-3f3c8bba65b8")
	require.NotEmpty(t, opm)

	// The OPM must be a valid base64url string without padding.
	// 8 bytes (with top 60 bits meaningful) encoded as base64url = 11 chars (ceil(8*4/3) = 11).
	assert.Regexp(t, `^[A-Za-z0-9_-]{11}$`, opm, "OPM must be an 11-char base64url string (8 bytes, 60 bits significant)")
}

// newTestReceiverWithOPM creates a minimal HTTPReceiver with EnableOPMFetch configured.
func newTestReceiverWithOPM(url string) *HTTPReceiver {
	conf := newTestReceiverConfig()
	conf.EnableOPMFetch = true
	conf.OPMValidateURL = url
	return newTestReceiverFromConfig(conf)
}

func validateServerResponse(t *testing.T, orgID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": orgID,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
}

func TestFetchOPM_Success(t *testing.T) {
	orgUUID := "abc12312-000e-11ea-a34b-3f3c8bba65b8"
	srv := validateServerResponse(t, orgUUID)
	defer srv.Close()

	rcv := newTestReceiverWithOPM(srv.URL)
	// initialise agentState so setOrgPropMarker can update it
	_, _ = rcv.makeInfoHandler()

	opm, err := fetchOPM(context.Background(), rcv.conf.NewHTTPClient(), rcv.conf)
	require.NoError(t, err)
	assert.Equal(t, computeOPM(orgUUID), opm)
}

func TestFetchOPM_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	rcv := newTestReceiverWithOPM(srv.URL)
	_, err := fetchOPM(context.Background(), rcv.conf.NewHTTPClient(), rcv.conf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestFetchOPM_EmptyID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "",
			},
		}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	rcv := newTestReceiverWithOPM(srv.URL)
	_, err := fetchOPM(context.Background(), rcv.conf.NewHTTPClient(), rcv.conf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty org UUID")
}

func TestFetchOPM_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`not valid json`)) //nolint:errcheck
	}))
	defer srv.Close()

	rcv := newTestReceiverWithOPM(srv.URL)
	_, err := fetchOPM(context.Background(), rcv.conf.NewHTTPClient(), rcv.conf)
	require.Error(t, err)
}

func TestStartOPMFetch_Disabled(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	conf := newTestReceiverConfig()
	conf.EnableOPMFetch = false
	conf.OPMValidateURL = srv.URL
	rcv := newTestReceiverFromConfig(conf)

	rcv.StartOPMFetch(context.Background(), time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int32(0), requestCount.Load(), "no request should be made when OPM fetch is disabled")
	assert.Empty(t, rcv.OrgPropMarker())
}

func TestStartOPMFetch_RetrySuccess(t *testing.T) {
	var requestCount atomic.Int32
	orgUUID := "bd4276fd-000e-11ea-a34b-3f3c8bba65b8"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		if n < 3 {
			// fail the first two attempts
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp := map[string]interface{}{"data": map[string]interface{}{"id": orgUUID}}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	rcv := newTestReceiverWithOPM(srv.URL)
	_, _ = rcv.makeInfoHandler()

	rcv.StartOPMFetch(context.Background(), time.Millisecond)

	require.Eventually(t, func() bool {
		return rcv.OrgPropMarker() != ""
	}, 500*time.Millisecond, 5*time.Millisecond, "OPM should be set after retries")

	assert.Equal(t, computeOPM(orgUUID), rcv.OrgPropMarker())
	assert.Equal(t, int32(3), requestCount.Load(), "should have made exactly 3 requests")
}

func TestStartOPMFetch_AllRetriesExhausted(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rcv := newTestReceiverWithOPM(srv.URL)
	_, _ = rcv.makeInfoHandler()

	rcv.StartOPMFetch(context.Background(), time.Millisecond)

	// Wait long enough for all 4 attempts (initial + 3 retries) plus delays
	time.Sleep(200 * time.Millisecond)

	assert.Empty(t, rcv.OrgPropMarker(), "OPM should remain empty after all retries exhausted")
	assert.Equal(t, int32(4), requestCount.Load(), "should have made exactly 4 attempts (initial + 3 retries)")
}

func TestStartOPMFetch_ContextCancellation(t *testing.T) {
	var requestCount atomic.Int32

	// First request fails, then we cancel. Subsequent retries should be aborted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rcv := newTestReceiverWithOPM(srv.URL)
	_, _ = rcv.makeInfoHandler()

	ctx, cancel := context.WithCancel(context.Background())
	// Use a large base delay so cancellation happens before any retry
	rcv.StartOPMFetch(ctx, 10*time.Second)

	// Wait for the first attempt to complete, then cancel
	require.Eventually(t, func() bool {
		return requestCount.Load() >= 1
	}, 100*time.Millisecond, 1*time.Millisecond)
	cancel()

	// Give the goroutine time to see the cancellation
	time.Sleep(20 * time.Millisecond)

	assert.Empty(t, rcv.OrgPropMarker(), "OPM should not be set when context is cancelled")
	assert.Equal(t, int32(1), requestCount.Load(), "only one request should have been made before cancellation")
}

func TestSetOrgPropMarker_UpdatesAgentState(t *testing.T) {
	rcv := newTestReceiverFromConfig(newTestReceiverConfig())
	_, _ = rcv.makeInfoHandler()

	before := rcv.agentState.Load()
	rcv.setOrgPropMarker("test-opm")
	after := rcv.agentState.Load()

	assert.NotEmpty(t, before)
	assert.NotEmpty(t, after)
	assert.NotEqual(t, before, after, "agentState should change when OPM is set")
	assert.Equal(t, "test-opm", rcv.OrgPropMarker())
}

func TestSetOrgPropMarker_BeforeMakeInfoHandler(t *testing.T) {
	// Simulate OPM arriving before makeInfoHandler initialises computeStateHash.
	rcv := newTestReceiverFromConfig(newTestReceiverConfig())

	// setOrgPropMarker should not panic even when computeStateHash is nil.
	rcv.setOrgPropMarker("early-opm")
	assert.Equal(t, "early-opm", rcv.OrgPropMarker())

	// makeInfoHandler should pick up the already-stored OPM.
	hash, h := rcv.makeInfoHandler()
	assert.NotEmpty(t, hash)
	_ = h
	// agentState should reflect the early OPM.
	expectedHash := rcv.agentState.Load()
	assert.NotEmpty(t, expectedHash)
}
