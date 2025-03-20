// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package noneimpl implements a noop version of the auth_token component
package noneimpl

import (
	"crypto/tls"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
)

type authToken struct {
}

var _ authtoken.Component = (*authToken)(nil)

// NewNoopAuthToken return a void implementation of the authtoken.Component
func NewNoopAuthToken() authtoken.Component {
	return &authToken{}
}

// Get returns the session token
func (at *authToken) Get() (string, error) {
	return "", nil
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSClientConfig() *tls.Config {
	return &tls.Config{}
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSServerConfig() *tls.Config {
	return &tls.Config{}
}
