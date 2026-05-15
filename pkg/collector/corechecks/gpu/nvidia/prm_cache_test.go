// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

func TestPRMCacheRefresh(t *testing.T) {
	socketPath, _ := startPRMTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/prm-metrics":
			_, _ = w.Write([]byte(`[{"request":{"device_uuid":"GPU-1","port":1,"group":34},"counters":{"nvlink.plr.rx.codes":123}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	cache := NewPRMCache()
	cache.client = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath))
	cache.RegisterRequest(model.PRMRequest{DeviceUUID: "GPU-1", Port: 1, Group: 34})

	require.NoError(t, cache.Refresh())
	counters, err := cache.GetCounters("GPU-1", 1)
	require.NoError(t, err)
	require.Equal(t, uint64(123), counters["nvlink.plr.rx.codes"])
}

func TestPRMCacheRefreshPartialError(t *testing.T) {
	socketPath, _ := startPRMTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/prm-metrics":
			_, _ = w.Write([]byte(`[
				{"request":{"device_uuid":"GPU-1","port":1,"group":34},"counters":{"nvlink.plr.rx.codes":123}},
				{"request":{"device_uuid":"GPU-1","port":2,"group":34},"error":"port unavailable"}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	cache := NewPRMCache()
	cache.client = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath))
	cache.RegisterRequest(model.PRMRequest{DeviceUUID: "GPU-1", Port: 1, Group: 34})
	cache.RegisterRequest(model.PRMRequest{DeviceUUID: "GPU-1", Port: 2, Group: 34})

	require.NoError(t, cache.Refresh())
	_, err := cache.GetCounters("GPU-1", 2)
	require.ErrorContains(t, err, "port unavailable")
}

func TestPRMCacheRefreshFailure(t *testing.T) {
	socketPath, _ := startPRMTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/prm-metrics":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	cache := NewPRMCache()
	cache.client = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath))
	cache.RegisterRequest(model.PRMRequest{DeviceUUID: "GPU-1", Port: 1, Group: 34})

	err := cache.Refresh()
	require.Error(t, err)
	_, err = cache.GetCounters("GPU-1", 1)
	require.Error(t, err)
}

func startPRMTestServer(t *testing.T, handler http.Handler) (string, *httptest.Server) {
	t.Helper()

	socketPath := testutil.SystemProbeSocketPath(t, "prm-cache")
	server, err := testutil.NewSystemProbeTestServer(handler, socketPath)
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Close)
	return socketPath, server
}
