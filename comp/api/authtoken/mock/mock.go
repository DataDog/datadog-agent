// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock is a mock implementation of the authtoken component
package mock

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/secureclient"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// inMemoryAuthComponent is a mock for the authtoken component
// It is used to set the auth token, client TLS config and server TLS config in memory
type inMemoryAuthComponent struct {
	t    testing.TB
	conf config.Component
}

// Get is a mock of the fetchonly Get function
func (m *inMemoryAuthComponent) Get() string {
	return util.GetAuthToken()
}

// GetTLSClientConfig is a mock of the fetchonly GetTLSClientConfig function
func (m *inMemoryAuthComponent) GetTLSClientConfig() *tls.Config {
	return util.GetTLSClientConfig()
}

// GetTLSServerConfig is a mock of the fetchonly GetTLSServerConfig function
func (m *inMemoryAuthComponent) GetTLSServerConfig() *tls.Config {
	return util.GetTLSServerConfig()
}

func (m *inMemoryAuthComponent) NewMockServer(handler http.Handler) *httptest.Server {
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

// Module returns a fx module that provides constructors for the optional and normal authtoken mock components
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func(t testing.TB) option.Option[authtoken.Component] { return New(t).Optional() }),
		fx.Provide(newMock),
	)
}

func newMock(deps option.Option[authtoken.Component]) (authtoken.Component, error) {
	auth, ok := deps.Get()
	if !ok {
		return nil, fmt.Errorf("auth token not found")
	}
	return auth, nil
}

// New returns a new authtoken mock
func New(t testing.TB) authtoken.Mock {
	// setting pkg/api/util globals
	util.SetAuthTokenInMemory(t) // TODO IPC: remove this line when the migration to component framework will be fully finished

	return &inMemoryAuthComponent{
		t:    t,
		conf: config.NewMock(t),
	}
}

// New returns a new authtoken mock
func (m *inMemoryAuthComponent) Optional() option.Option[authtoken.Component] {
	return option.New[authtoken.Component](m)
}

func (m *inMemoryAuthComponent) HTTPMiddleware(next http.Handler) http.Handler {
	return authtoken.NewHTTPMiddleware(m.t.Logf, m.Get())(next)
}

func (m *inMemoryAuthComponent) GetClient(_ ...authtoken.ClientOption) authtoken.SecureClient {
	return secureclient.NewClient(m.Get(), m.GetTLSClientConfig(), m.conf)
}
