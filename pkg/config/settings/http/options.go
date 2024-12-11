// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http implements helpers for the runtime settings HTTP API
package http

import "github.com/DataDog/datadog-agent/pkg/api/util"

// ClientOptions holds options for the HTTP client
type ClientOptions struct {
	CloseConnection util.ShouldCloseConnection
}

// NewHTTPClientOptions returns a new struct containing the HTTP client options
func NewHTTPClientOptions(closeConnection util.ShouldCloseConnection) ClientOptions {
	return ClientOptions{closeConnection}
}
