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
	"time"

	"github.com/stretchr/testify/require"

	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

func TestDriverEventsCacheRefresh(t *testing.T) {
	requests := 0
	socketPath, _ := startDriverEventsTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/driver-events":
			requests++
			if requests == 1 {
				_, _ = w.Write([]byte(`[{"device_uuid":"GPU-1","timestamp":"2026-01-01T00:00:00Z","type":"nvidia_xid","nvidia_xid":{"xid_code":31}}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	cache := NewDriverEventsCache(sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath)))

	require.NoError(t, cache.Refresh())
	events := cache.Get()
	require.Len(t, events, 1)
	require.Equal(t, "GPU-1", events[0].DeviceUUID)
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), events[0].Timestamp)

	require.NoError(t, cache.Refresh())
	require.Empty(t, cache.Get())
}

func startDriverEventsTestServer(t *testing.T, handler http.Handler) (string, *httptest.Server) {
	t.Helper()

	socketPath := testutil.SystemProbeSocketPath(t, "driver-events-cache")
	server, err := testutil.NewSystemProbeTestServer(handler, socketPath)
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Close)
	return socketPath, server
}
