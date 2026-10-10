// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

// testUploader returns an uploader pointed at a server that answers each
// request with the next status in statuses, and a count of requests served.
func testUploader(t *testing.T, statuses ...int) (*networkPcapUploader, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i := int(calls.Add(1)) - 1
		w.WriteHeader(statuses[min(i, len(statuses)-1)])
	}))
	t.Cleanup(srv.Close)
	return &networkPcapUploader{url: srv.URL + networkPcapPath, httpClient: srv.Client()}, &calls
}

func TestUploadRetriesThrottlingAndServerErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			u, calls := testUploader(t, status, http.StatusAccepted)
			require.NoError(t, u.Upload(context.Background(), []byte("pcap"), "capture-id"))
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestUploadDoesNotRetryClientErrors(t *testing.T) {
	u, calls := testUploader(t, http.StatusBadRequest)
	require.Error(t, u.Upload(context.Background(), []byte("pcap"), "capture-id"))
	assert.EqualValues(t, 1, calls.Load())
}

func TestNewUploaderUsesAgentTransport(t *testing.T) {
	transport := &http.Transport{}
	u := newNetworkPcapUploader(&config.Config{AgentHTTPClient: &http.Client{Transport: transport}})
	assert.Same(t, transport, u.httpClient.Transport)
}
