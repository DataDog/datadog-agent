// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package mock

import (
	"sync"
	"testing"

	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// The following certificate and key are used for testing purposes only.
// They have been generated using the following command:
//
//	openssl req -x509 -newkey ec:<(openssl ecparam -name prime256v1) -keyout key.pem -out cert.pem -days 3650 \
//	  -subj "/O=Datadog, Inc." \
//	  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
//	  -addext "keyUsage=digitalSignature,keyEncipherment" \
//	  -addext "extendedKeyUsage=serverAuth,clientAuth" \
//	  -addext "basicConstraints=CA:TRUE" \
//	  -nodes
var (
	testIPCCert = []byte(`-----BEGIN CERTIFICATE-----
MIIBzDCCAXKgAwIBAgIUR2IeG+dUuibzpp5+uNvk/4g6M+cwCgYIKoZIzj0EAwIw
GDEWMBQGA1UECgwNRGF0YWRvZywgSW5jLjAeFw0yNTAzMjQxMzM2NDlaFw0zNTAz
MjIxMzM2NDlaMBgxFjAUBgNVBAoMDURhdGFkb2csIEluYy4wWTATBgcqhkjOPQIB
BggqhkjOPQMBBwNCAARt8T/DyYsxBbDsSJJY2drHbFoTWYT9u1gzgzooDbbLBzuj
PHqwmdNHOShuNLSgVkIjIkmZgKendRYgu3uXoswgo4GZMIGWMB0GA1UdDgQWBBQa
FF5ne0D5vg89fbLm/xUqHGEQvjAfBgNVHSMEGDAWgBQaFF5ne0D5vg89fbLm/xUq
HGEQvjAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8AAAEwCwYDVR0PBAQDAgWgMB0G
A1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAMBgNVHRMEBTADAQH/MAoGCCqG
SM49BAMCA0gAMEUCIQCCLOBCW7yF9LkNAzuGbgrZSH1GklnrJWNGcN2XsspEnQIg
TniyxGyuEhHLJkB5LA1N+Q0NKIwjMnb8/Aw7Z1NIolU=
-----END CERTIFICATE-----
`)
	testIPCKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg1wUA94nU4LmF81zw
tAaSSpKwY9fI1AXbj1Nr94XW+lyhRANCAARt8T/DyYsxBbDsSJJY2drHbFoTWYT9
u1gzgzooDbbLBzujPHqwmdNHOShuNLSgVkIjIkmZgKendRYgu3uXoswg
-----END PRIVATE KEY-----
`)
	initTLS                          sync.Once
	tlsClientConfig, tlsServerConfig *tls.Config
	token                            = "test-token"
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

	config := configmock.New(t)

	// Initialize the TLS configs only once
	initTLS.Do(func() {
		var err error
		tlsClientConfig, tlsServerConfig, err = cert.GetTLSConfigFromCert(testIPCCert, testIPCKey)
		require.NoError(t, err)
	})

	return &IPCMock{
		t:      t,
		conf:   config,
		client: ipchttp.NewClient(token, tlsClientConfig, config),
	}
}

// GetAuthToken is a mock of the fetchonly GetAuthToken function
func (m *IPCMock) GetAuthToken() string {
	return token
}

// GetTLSClientConfig is a mock of the fetchonly GetTLSClientConfig function
func (m *IPCMock) GetTLSClientConfig() *tls.Config {
	return tlsClientConfig.Clone()
}

// GetTLSServerConfig is a mock of the fetchonly GetTLSServerConfig function
func (m *IPCMock) GetTLSServerConfig() *tls.Config {
	return tlsServerConfig.Clone()
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
