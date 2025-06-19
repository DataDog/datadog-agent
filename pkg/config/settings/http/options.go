// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http implements helpers for the runtime settings HTTP API
package http

// ShouldCloseConnection is an option to DoGet to indicate whether to close the underlying
// connection after reading the response
type ShouldCloseConnection int

const (
	// LeaveConnectionOpen keeps the underlying connection open after reading the request response
	LeaveConnectionOpen ShouldCloseConnection = iota
	// CloseConnection closes the underlying connection after reading the request response
	CloseConnection
)

// ClientOptions holds options for the HTTP client
type ClientOptions struct {
	CloseConnection ShouldCloseConnection
}

// NewHTTPClientOptions returns a new struct containing the HTTP client options
func NewHTTPClientOptions(closeConnection ShouldCloseConnection) ClientOptions {
	return ClientOptions{closeConnection}
}
