// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !windows

// Package flare implements 'agent flare'.
package flare

import (
	"net/http"
	"net/http/httptest"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// NewSystemProbeTestServer starts a new mock server to handle System Probe requests.
func NewSystemProbeTestServer(_ http.Handler) (*httptest.Server, error) {
	// Linux still uses a port-based system-probe, it does not need a dedicated server
	// for the tests.
	return nil, nil
}

// InjectConnectionFailures injects a failure in TestReadProfileDataErrors.
func InjectConnectionFailures(_ model.Config, _ model.Config) {
}

// ClearConnectionFailures clears the injected failure in TestReadProfileDataErrors.
func ClearConnectionFailures(_ model.Config, _ model.Config) {
}
