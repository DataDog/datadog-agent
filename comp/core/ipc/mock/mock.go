// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock is a mock implementation of the IPC component
package mock

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/ipc"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// inMemoryIPCComponent is a mock for the IPC component
// It is used to set the auth token, client TLS config and server TLS config in memory
type inMemoryIPCComponent struct {
	t    testing.TB
	conf config.Component
}

// Get is a mock of the fetchonly Get function
func (m *inMemoryIPCComponent) Get() string {
	return util.GetAuthToken()
}

// GetTLSClientConfig is a mock of the fetchonly GetTLSClientConfig function
func (m *inMemoryIPCComponent) GetTLSClientConfig() *tls.Config {
	return util.GetTLSClientConfig()
}

// GetTLSServerConfig is a mock of the fetchonly GetTLSServerConfig function
func (m *inMemoryIPCComponent) GetTLSServerConfig() *tls.Config {
	return util.GetTLSServerConfig()
}

func (m *inMemoryIPCComponent) NewMockServer(handler http.Handler) *httptest.Server {
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
		fx.Provide(func(t testing.TB) ipc.Component { return New(t) }),
		fxutil.ProvideOptional[ipc.Component](),
	)
}

// New returns a new authtoken mock
func New(t testing.TB) ipc.Mock {
	// setting pkg/api/util globals
	util.SetAuthTokenInMemory(t) // TODO IPC: remove this line when the migration to component framework will be fully finished

	return &inMemoryIPCComponent{
		t:    t,
		conf: config.NewMock(t),
	}
}

// New returns a new authtoken mock
func (m *inMemoryIPCComponent) Optional() option.Option[ipc.Component] {
	return option.New[ipc.Component](m)
}
