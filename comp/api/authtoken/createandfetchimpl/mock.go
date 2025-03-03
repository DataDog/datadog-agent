// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package createandfetchimpl

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokeninterface "github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

// MockAuthToken is a mock for fetch only authtoken
type MockAuthToken struct{}

// Get is a mock of the fetchonly Get function
func (fc *MockAuthToken) Get() string {
	return "a string"
}

// GetTLSClientConfig is a mock of the fetchonly GetTLSClientConfig function
func (fc *MockAuthToken) GetTLSClientConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
	}
}

// GetTLSServerConfig is a mock of the fetchonly GetTLSServerConfig function
func (fc *MockAuthToken) GetTLSServerConfig() *tls.Config {
	// Starting a TLS httptest server to retrieve a localhost tlsCert
	ts := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	tlsConfig := ts.TLS.Clone()
	ts.Close()

	return tlsConfig
}

func (_ *MockAuthToken) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

type mockSecureClient struct {
	http.Client
}

func (m *MockAuthToken) GetClient(_ ...authtoken.ClientOption) authtokeninterface.SecureClient {
	return &secureClient{authToken: m.Get()}
}

// NewMock returns a new fetch only authtoken mock
func newMock() authtokeninterface.Component {
	return &MockAuthToken{}
}
