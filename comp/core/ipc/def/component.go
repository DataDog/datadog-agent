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
	"time"
)

// team: agent-runtimes

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

// RequestParams is a struct that contains the parameters for a request
type RequestParams struct {
	*http.Request
	Timeout time.Duration
}

// RequestOption allows to specify custom behavior for requests
type RequestOption func(req *RequestParams)

// HTTPClient is a HTTP client that abstracts communications between Agent processes.
// It handles TLS configuration and authentication token management for inter-process communication.
type HTTPClient interface {
	// Do sends an HTTP request, handling IPC authentication and TLS, and returns the response body.
	// It wraps the standard http.Client.Do method.
	Do(req *http.Request, opts ...RequestOption) (resp []byte, err error)
	// Get sends a GET request, handling IPC authentication and TLS, and returns the response body.
	// It wraps the standard http.Client.Get method.
	Get(url string, opts ...RequestOption) (resp []byte, err error)
	// Head sends a HEAD request, handling IPC authentication and TLS, and returns the response.
	// It wraps the standard http.Client.Head method.
	Head(url string, opts ...RequestOption) (resp []byte, err error)
	// Post sends a POST request with the given body and content type, handling IPC authentication and TLS,
	// and returns the response body.
	// It wraps the standard http.Client.Post method.
	Post(url string, contentType string, body io.Reader, opts ...RequestOption) (resp []byte, err error)
	// PostChunk sends a POST request with a chunked body, handling IPC authentication and TLS.
	// The provided callback function is called for each chunk received in the response.
	// It wraps the standard http.Client.Post method.
	PostChunk(url string, contentType string, body io.Reader, onChunk func([]byte), opts ...RequestOption) (err error)
	// PostForm sends a POST request with form data, handling IPC authentication and TLS,
	// and returns the response body.
	// It wraps the standard http.Client.PostForm method.
	PostForm(url string, data url.Values, opts ...RequestOption) (resp []byte, err error)
	// NewIPCEndpoint creates a new IPC endpoint client for the specified path.
	// This allows making requests to a specific endpoint path without repeatedly specifying it.
	NewIPCEndpoint(endpointPath string) (Endpoint, error)
}

// Endpoint represents a specific endpoint of an IPC server, allowing pre-configured requests.
type Endpoint interface {
	// DoGet sends a GET request to the pre-configured endpoint path.
	DoGet(options ...RequestOption) ([]byte, error)
}
