// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
)

// Receiver defines the lifecycle and common capabilities for any trace receiver
// implementation. Receivers are responsible for exposing transport-specific
// endpoints (HTTP, gRPC, FFI, etc.) and delegating domain processing to
// transport-agnostic functions.
type Receiver interface {
	// Start initializes and starts the receiver server(s) and background loops.
	Start()

	// Stop gracefully stops the receiver and all related background routines.
	Stop() error

	// BuildHandlers ensures receiver handlers are built and available
	// for composition in higher-level components.
	BuildHandlers()

	// UpdateAPIKey refreshes any internal state depending on the API key
	// (e.g. proxied endpoints) without requiring a full restart.
	UpdateAPIKey()

	// Languages returns the list of languages seen by the receiver.
	Languages() string

	// GetStats returns the ReceiverStats for tests and internal consumers.
	GetStats() *info.ReceiverStats

	// GetHandler returns an http.Handler for the given pattern if present.
	GetHandler(pattern string) (http.Handler, bool)
}
