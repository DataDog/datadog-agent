// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build unix

// Package testutil contains test utilities for the system-probe API server.
package testutil

import (
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
)

// SystemProbeSocketPath returns a temporary socket path for testing.
func SystemProbeSocketPath(t *testing.T, _ string) string {
	return path.Join(t.TempDir(), "sysprobe.sock")
}

// NewSystemProbeTestServer starts a new mock server to handle System Probe requests.
func NewSystemProbeTestServer(handler http.Handler, socketPath string) (*httptest.Server, error) {
	unixServer := httptest.NewUnstartedServer(handler)
	var err error
	unixServer.Listener, err = server.NewListener(socketPath)
	if err != nil {
		return nil, err
	}
	return unixServer, nil
}
