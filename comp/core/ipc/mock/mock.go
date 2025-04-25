// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// IPCMock is a mock for the IPC component
// It is used to set the auth token, client TLS config and server TLS config in memory
type IPCMock struct {
	ipc.Component
	t      testing.TB
	conf   config.Component
	client ipc.HTTPClient
}

// New returns a mock for ipc component.
func New(t testing.TB) *IPCMock {
	// setting pkg/api/util globals
	util.SetAuthTokenInMemory(t) // TODO IPC: remove this line when the migration to component framework will be fully finished

	config := configmock.New(t)

	return &IPCMock{
		t:      t,
		conf:   config,
		client: ipchttp.NewClient(util.GetAuthToken(), util.GetTLSClientConfig(), config),
	}
}

// GetAuthToken is a mock of the fetchonly GetAuthToken function
func (m *IPCMock) GetAuthToken() string {
	return util.GetAuthToken()
}

// GetTLSClientConfig is a mock of the fetchonly GetTLSClientConfig function
func (m *IPCMock) GetTLSClientConfig() *tls.Config {
	return util.GetTLSClientConfig()
}

// GetTLSServerConfig is a mock of the fetchonly GetTLSServerConfig function
func (m *IPCMock) GetTLSServerConfig() *tls.Config {
	return util.GetTLSServerConfig()
}

// HTTPMiddleware is a mock of the ipc.Component HTTPMiddleware method
func (m *IPCMock) HTTPMiddleware(next http.Handler) http.Handler {
	return ipchttp.NewHTTPMiddleware(m.t.Logf, m.GetAuthToken())(next)
}

// GetClient is a mock of the ipc.Component GetClient method
func (m *IPCMock) GetClient() ipc.HTTPClient {
	return m.client
}

// NewMockServer allows to create a mock server that use the IPC certificate
func (m *IPCMock) NewMockServer(handler http.Handler) *httptest.Server {
	ts := httptest.NewUnstartedServer(handler)
	ts.TLS = m.GetTLSServerConfig()
	ts.StartTLS()
	m.t.Cleanup(ts.Close)

	m.t.Logf("Starting mock server at %v", ts.URL)

	// set the cmd_host and cmd_port in the config
	addr, err := url.Parse(ts.URL)
	require.NoError(m.t, err)
	localHost, localPort, _ := net.SplitHostPort(addr.Host)
	m.conf.Set("cmd_host", localHost, pkgconfigmodel.SourceAgentRuntime)
	m.conf.Set("cmd_port", localPort, pkgconfigmodel.SourceAgentRuntime)
	m.t.Cleanup(func() {
		m.conf.UnsetForSource("cmd_host", pkgconfigmodel.SourceAgentRuntime)
		m.conf.UnsetForSource("cmd_port", pkgconfigmodel.SourceAgentRuntime)
	})

	return ts
}
