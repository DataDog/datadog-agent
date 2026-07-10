// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package backend holds files related to forwarder backends for security profiles
package backend

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestMRFFailoverActive(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		failoverLogs bool
		want         bool
	}{
		{name: "mrf disabled", enabled: false, failoverLogs: false, want: false},
		{name: "enabled but no active failover", enabled: true, failoverLogs: false, want: false},
		{name: "failover flag set but mrf disabled", enabled: false, failoverLogs: true, want: false},
		{name: "enabled and failing over", enabled: true, failoverLogs: true, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("multi_region_failover.enabled", tc.enabled)
			cfg.SetInTest("multi_region_failover.failover_logs", tc.failoverLogs)

			assert.Equal(t, tc.want, mrfFailoverActive(cfg))
		})
	}
}

// newCountingServer returns an httptest server that accepts every request (202 Accepted, the
// status sendToEndpoint treats as success) and increments hits on each call.
func newCountingServer(hits *atomic.Int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Inc()
		w.WriteHeader(http.StatusAccepted)
	}))
}

// endpointForServer builds a plain-HTTP Endpoint targeting the given test server so that
// GetEndpointURL produces an http:// URL matching httptest.
func endpointForServer(t *testing.T, srv *httptest.Server, apiKeyPath string) logsconfig.Endpoint {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return logsconfig.NewEndpoint("api-key", apiKeyPath, host, port, "", false /* useSSL */)
}

func TestHandleActivityDumpGatesMRFEndpointOnFailover(t *testing.T) {
	primaryHits := atomic.NewInt64(0)
	mrfHits := atomic.NewInt64(0)

	primarySrv := newCountingServer(primaryHits)
	defer primarySrv.Close()
	mrfSrv := newCountingServer(mrfHits)
	defer mrfSrv.Close()

	primaryEp := endpointForServer(t, primarySrv, "api_key")
	mrfEp := endpointForServer(t, mrfSrv, "multi_region_failover.api_key")
	mrfEp.IsMRF = true

	backend := &ActivityDumpRemoteBackend{
		tooLargeEntities: atomic.NewUint64(0),
		client:           primarySrv.Client(),
		endpoints:        logsconfig.NewEndpoints(primaryEp, []logsconfig.Endpoint{mrfEp}, false, true),
	}

	cfg := configmock.New(t)
	cfg.SetInTest("multi_region_failover.enabled", true)

	// No active failover: the MRF endpoint must be skipped while the primary still receives the dump.
	cfg.SetInTest("multi_region_failover.failover_logs", false)
	require.NoError(t, backend.HandleActivityDump("image", "tag", []byte(`{}`), []byte("dump")))
	assert.Equal(t, int64(1), primaryHits.Load(), "primary endpoint should always receive the dump")
	assert.Equal(t, int64(0), mrfHits.Load(), "MRF endpoint must not be used outside of an active failover")

	// Remote Configuration switches the failover on: the MRF endpoint now receives the dump too.
	cfg.SetInTest("multi_region_failover.failover_logs", true)
	require.NoError(t, backend.HandleActivityDump("image", "tag", []byte(`{}`), []byte("dump")))
	assert.Equal(t, int64(2), primaryHits.Load())
	assert.Equal(t, int64(1), mrfHits.Load(), "MRF endpoint should receive the dump while failing over")
}
