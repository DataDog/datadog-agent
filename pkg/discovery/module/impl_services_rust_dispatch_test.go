// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && linux_bpf && dd_discovery_rust && cgo

package module

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// setupCGORustDiscoveryModule creates the same in-process Go module as
// setupGoDiscoveryModule, but forces the Rust CGo backend (getServicesRust) to
// be used instead of the Go backend.  This exercises impl_services_rust_linux.go
// and its CGo translation helpers.
func setupCGORustDiscoveryModule(t *testing.T) *testDiscoveryModule {
	t.Helper()
	old := rustBackendEnabled
	rustBackendEnabled = true
	t.Cleanup(func() { rustBackendEnabled = old })
	return setupGoDiscoveryModule(t)
}

// TestDiscoveryRustCGOBackend runs the full discoveryTestSuite against the CGo
// backend (libdd_discovery) to ensure getServicesRust is exercised by tests.
// This test only compiles and runs when dd_discovery_rust && cgo are active,
// i.e. when libdd_discovery.a has been built and linked in.
func TestDiscoveryRustCGOBackend(t *testing.T) {
	suite.Run(t, &discoveryTestSuite{
		setupModule:            setupCGORustDiscoveryModule,
		expectedImplementation: "system-probe",
	})
}
