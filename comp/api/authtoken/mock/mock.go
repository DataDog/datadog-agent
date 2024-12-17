// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the authtoken component
package mock

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	authtoken "github.com/DataDog/datadog-agent/comp/api/authtoken/def"
)

type mock struct{}

// Mock returns a mock for authtoken component.
func Mock(t *testing.T) authtoken.Component {
	return &mock{}
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
