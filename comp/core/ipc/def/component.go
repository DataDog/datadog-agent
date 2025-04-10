// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ipc takes care of the IPC artifacts lifecycle (creation, loading, deletion of auth_token, IPC certificate, IPC key).
// It also provides helpers to use them in the agent (TLS configuration, HTTP client, etc.).
package ipc

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
)

// team: agent-runtimes

// Params defines the parameters for this component.
type Params struct {
	// AllowWriteArtifacts is a boolean that determines whether the component should allow writing auth artifacts on file system
	// or only read them.
	AllowWriteArtifacts bool
}

// ForDaemon returns the params for the daemon
// It allows the Agent to write the IPC artifacts on the file system
func ForDaemon() Params {
	return Params{
		AllowWriteArtifacts: true,
	}
}

// ForOneShot returns the params for the one-shot commands
// It only allows the Agent to read the IPC artifacts from the file system
func ForOneShot() Params {
	return Params{
		AllowWriteArtifacts: false,
	}
}

// Component is the component type.
type Component interface {
	// GetAuthToken returns the session token
	GetAuthToken() string
	// GetTLSClientConfig returns a copy of the TLS configuration for HTTPS clients
	GetTLSClientConfig() *tls.Config
	// GetTLSServerConfig returns a copy of the TLS configuration for HTTPS servers
	GetTLSServerConfig() *tls.Config
	// HTTPMiddleware returns a middleware that verifies the auth_token in incoming HTTP requests
	HTTPMiddleware(next http.Handler) http.Handler
	// GetClient returns an HTTP client that verifies the certificate of the server and includes the auth_token in outgoing requests
	GetClient() HTTPClient
}

// RequestOption allows to specify custom behavior for requests
type RequestOption func(req *http.Request, onEnding func(func())) *http.Request

// HTTPClient is a HTTP client that abstracts communications between Agent processes
type HTTPClient interface {
	Do(req *http.Request, opts ...RequestOption) (resp []byte, err error)
	Get(url string, opts ...RequestOption) (resp []byte, err error)
	Head(url string, opts ...RequestOption) (resp []byte, err error)
	Post(url string, contentType string, body io.Reader, opts ...RequestOption) (resp []byte, err error)
	PostChunk(url string, contentType string, body io.Reader, onChunk func([]byte), opts ...RequestOption) (err error)
	PostForm(url string, data url.Values, opts ...RequestOption) (resp []byte, err error)
	NewIPCEndpoint(endpointPath string) (Endpoint, error)
}

// Endpoint represents a specific endpoint of an IPC server
type Endpoint interface {
	DoGet(options ...RequestOption) ([]byte, error)
}
