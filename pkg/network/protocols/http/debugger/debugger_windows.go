// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package debugger provides utilities for testing the HTTP protocol.
package debugger

import (
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"net/http"
)

// GetHTTPDebugEndpoint returns a handler for debugging HTTP requests.
func GetHTTPDebugEndpoint(tracer *tracer.Tracer) func(http.ResponseWriter, *http.Request) {
	return nil
}
