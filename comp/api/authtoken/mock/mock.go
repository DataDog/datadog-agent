// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package mock provides a mock for the authtoken component
package mock

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"

	authtoken "github.com/DataDog/datadog-agent/comp/api/authtoken/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mock struct{}

// Module is a module containing the mock, useful for testing
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(Mock),
	)
}

// Provides defines the output of the authtoken mock component
type Provides struct {
	Comp authtoken.Component
}

// Mock returns a mock for authtoken component.
func Mock() Provides {
	return Provides{
		Comp: &mock{},
	}
}

// Get is a mock of the fetchonly Get function
func (fc *mock) Get() string {
	return "a string"
}

// GetTLSClientConfig is a mock of the fetchonly GetTLSClientConfig function
// It return a TLS configuration that disable server certificate check
func (fc *mock) GetTLSClientConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
	}
}

// GetTLSServerConfig is a mock of the fetchonly GetTLSServerConfig function
// It return a TLS configuration that contains a self-signed certificate for localhost only
func (fc *mock) GetTLSServerConfig() *tls.Config {
	// Starting a TLS httptest server to retrieve a localhost tlsCert
	ts := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	tlsConfig := ts.TLS.Clone()
	ts.Close()

	return tlsConfig
}
