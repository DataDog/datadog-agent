// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Runtime dispatch between the Go and Rust-library backends for getServices.
// Only compiled when dd_discovery_rust and cgo are both available, so that both
// getServicesGo (impl_linux.go, always present on Linux) and getServicesRust
// (impl_services_rust_linux.go) are available to call.

//go:build dd_discovery_rust && cgo

package module

import (
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// getServices dispatches to either the Go or Rust backend depending on the
// discovery.use_rust_library configuration. It acquires the module lock for
// the duration of the call.
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.config.UseRustLibrary {
		return s.getServicesRust(params)
	}
	return s.getServicesGo(params)
}
