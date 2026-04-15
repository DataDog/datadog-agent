// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Tests for the Go+RustLibrary backend (discovery.use_rust_library = true).
// Only compiled when dd_discovery_rust and cgo are both available.

//go:build test && linux_bpf && dd_discovery_rust && cgo

package module

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gorillamux "github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// setupRustLibDiscoveryModule creates a discovery module backed by the Rust library
// (discovery.use_rust_library = true). It overrides the config after construction
// so that the in-process module uses the Rust CGo backend regardless of what the
// global system-probe config says.
func setupRustLibDiscoveryModule(t *testing.T) *testDiscoveryModule {
	t.Helper()

	mux := gorillamux.NewRouter()

	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	d := mod.(*discovery)
	d.config.UseRustLibrary = true

	err = d.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	require.NoError(t, err)
	t.Cleanup(d.Close)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testDiscoveryModule{
		url:    srv.URL,
		client: http.DefaultClient,
	}
}

// TestDiscoveryRustLibrary runs the discovery test suite against the Rust-library backend.
func TestDiscoveryRustLibrary(t *testing.T) {
	suite.Run(t, &discoveryTestSuite{setupModule: setupRustLibDiscoveryModule})
}
