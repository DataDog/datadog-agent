// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http implements helpers for the runtime settings HTTP API
package http

import "github.com/DataDog/datadog-agent/pkg/api/util"

type HTTPClientOptions struct {
	closeConnection util.ShouldCloseConnection
}

func NewHTTPClientOptions(closeConnection util.ShouldCloseConnection) HTTPClientOptions {
	return HTTPClientOptions{closeConnection}
}
