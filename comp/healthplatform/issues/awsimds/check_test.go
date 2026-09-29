// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package awsimds

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// withProbeTarget points the probe at a local listener with short timeouts.
func withProbeTarget(t *testing.T, addr string) {
	t.Helper()
	originalAddress, originalDialTimeout, originalResponseTimeout := imdsAddress, dialTimeout, responseTimeout
	imdsAddress = addr
	dialTimeout, responseTimeout = 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() {
		imdsAddress, dialTimeout, responseTimeout = originalAddress, originalDialTimeout, originalResponseTimeout
	})
}

// setupCheck points the probe at a local listener and builds a module (env gate lives in NewModule).
func setupCheck(t *testing.T, addr string) *awsIMDSModule {
	t.Helper()
	withProbeTarget(t, addr)
	return testModule(t, "test-host")
}

func unresponsiveServer(t *testing.T) *httptest.Server {
	t.Helper()
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-stop
	}))
	t.Cleanup(func() {
		close(stop)
		srv.Close()
	})
	return srv
}

func TestCheck_HopLimitTooLow(t *testing.T) {
	srv := unresponsiveServer(t)
	m := setupCheck(t, srv.Listener.Addr().String())
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, m.instanceIssueID(), reports[0].IssueID)
	assert.Equal(t, IssueName, reports[0].IssueName)
	assert.Equal(t, imdsAddress, reports[0].Context[contextKeyIMDSAddress])
	assert.Equal(t, "false", reports[0].Context[contextKeyHostnameConfigured])
	assert.Equal(t, []string{"aws", "imds", "hop-limit", "container"}, reports[0].Tags)
}

// TestCheck_HostnameConfigured flags the report when DD_HOSTNAME is set so severity can be lowered.
func TestCheck_HostnameConfigured(t *testing.T) {
	srv := unresponsiveServer(t)
	withProbeTarget(t, srv.Listener.Addr().String())

	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("h"))
	cfg := configmock.New(t)
	cfg.SetInTest("hostname", "explicit-host")
	m := newModule(issues.ModuleDeps{Hostname: hn, Config: cfg})

	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "true", reports[0].Context[contextKeyHostnameConfigured])
}

// routedServer answers GET (IMDSv1) and PUT (IMDSv2 token) with fixed status codes.
func routedServer(t *testing.T, getStatus, putStatus int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(getStatus)
			return
		}
		w.WriteHeader(putStatus)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCheck_IMDSv2Reachable: the token PUT succeeds, so metadata is reachable and no issue is reported.
func TestCheck_IMDSv2Reachable(t *testing.T) {
	srv := routedServer(t, http.StatusInternalServerError, http.StatusOK)
	m := setupCheck(t, srv.Listener.Addr().String())
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// TestCheck_IMDSv1FallbackWorks covers the optional-token case (incl. hop limit 1): the token PUT
// fails but the agent's IMDSv1 GET fallback succeeds, so metadata is available and no issue fires.
func TestCheck_IMDSv1FallbackWorks(t *testing.T) {
	srv := routedServer(t, http.StatusOK, http.StatusInternalServerError)
	m := setupCheck(t, srv.Listener.Addr().String())
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// TestCheck_IMDSv2RequiredHopLimit: the token PUT is dropped by the hop limit and the IMDSv1 GET
// fallback is disabled (401), so the agent cannot retrieve metadata and the issue is reported.
func TestCheck_IMDSv2RequiredHopLimit(t *testing.T) {
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		<-stop // token PUT response dropped by the hop limit
	}))
	t.Cleanup(func() { close(stop); srv.Close() })
	m := setupCheck(t, srv.Listener.Addr().String())
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)
}

// TestCheck_IMDSBlocked: an intermediary (e.g. Kube2IAM/kiam) rejects both IMDSv1 and the token PUT.
func TestCheck_IMDSBlocked(t *testing.T) {
	srv := routedServer(t, http.StatusForbidden, http.StatusForbidden)
	m := setupCheck(t, srv.Listener.Addr().String())
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)
}

func TestCheck_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, ln.Close())
	m := setupCheck(t, ln.Addr().String())
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_DialTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	m := setupCheck(t, ln.Addr().String())
	// An expired dial deadline deterministically exercises the connection-timeout path.
	dialTimeout = -time.Second
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_ProxyIgnored(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	for _, respond := range []bool{true, false} {
		t.Run(fmt.Sprintf("respond=%t", respond), func(t *testing.T) {
			reached := make(chan struct{}, 1)
			stop := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached <- struct{}{}
				if respond {
					w.WriteHeader(http.StatusOK)
					return
				}
				<-stop
			}))
			defer srv.Close()
			defer close(stop)
			m := setupCheck(t, srv.Listener.Addr().String())
			reports, err := m.BuiltInStartupHealthCheck().Fn()
			require.NoError(t, err)
			select {
			case <-reached:
			default:
				t.Fatal("probe did not reach the listener directly")
			}
			if respond {
				assert.Empty(t, reports)
			} else {
				assert.Len(t, reports, 1)
			}
		})
	}
}
