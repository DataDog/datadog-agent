// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package testutil contains test utilities for the system-probe API server.
package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sysprobeserver "github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
)

// SystemProbeSocketPath returns a temporary named pipe for testing.
func SystemProbeSocketPath(_ *testing.T, name string) string {
	return `\\.\pipe\dd_system_probe_test_` + name
}

// NewSystemProbeTestServer starts a new mock server to handle System Probe requests.
func NewSystemProbeTestServer(handler http.Handler, socketPath string) (*httptest.Server, error) {
	server := httptest.NewUnstartedServer(handler)

	// The test named pipe allows the current user.
	conn, err := sysprobeserver.NewListenerForCurrentUser(socketPath)
	if err != nil {
		return nil, err
	}

	server.Listener = conn
	return server, nil
}
