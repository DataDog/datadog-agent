// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock is a mock implementation of the authtoken component
package mock

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	authtokeninterface "github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/pkg/api/util"
)

// inMemoryAuthComponent is a mock for the authtoken component
// It is used to set the auth token, client TLS config and server TLS config in memory
type inMemoryAuthComponent struct {
	t testing.TB
}

// Get is a mock of the fetchonly Get function
func (m *inMemoryAuthComponent) Get() (string, error) {
	return util.GetAuthToken(), nil
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
	return ts
}

// New returns a new authtoken mock
func New(t testing.TB) authtokeninterface.Mock {
	// setting pkg/api/util globals
	util.SetAuthTokenInMemory(t) // TODO IPC: remove this line when the migration to component framework will be fully finished

	return &inMemoryAuthComponent{
		t: t,
	}
}
