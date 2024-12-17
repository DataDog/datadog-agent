// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package noneimpl implements the authtoken component interface
package noneimpl

import (
	"crypto/tls"

	authtoken "github.com/DataDog/datadog-agent/comp/api/authtoken/def"
)

type authToken struct {
}

var _ authtoken.Component = (*authToken)(nil)

// Requires defines the dependencies for the authtoken component
type Requires struct {
}

// Provides defines the output of the authtoken component
type Provides struct {
	Comp authtoken.Component
}

// NewComponent creates a new authtoken component
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: &authToken{}}
}

// Get returns the session token
func (at *authToken) Get() string {
	return ""
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSClientConfig() *tls.Config {
	return &tls.Config{}
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSServerConfig() *tls.Config {
	return &tls.Config{}
}
