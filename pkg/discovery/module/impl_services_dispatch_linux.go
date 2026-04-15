// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build dd_discovery_rust && cgo

package module

import (
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// rustBackendEnabled is set once at startup from the environment variable.
var rustBackendEnabled = os.Getenv("DD_SERVICE_DISCOVERY_USE_RUST_BACKEND") == "true"

// logBackendOnce logs which backend is active the first time getServices is
// called, after the agent logger is fully initialized.
var logBackendOnce sync.Once

// getServices dispatches to the Rust or Go implementation based on the runtime
// configuration. Both implementations are compiled into the binary; the choice
// is made at startup via DD_SERVICE_DISCOVERY_USE_RUST_BACKEND.
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	logBackendOnce.Do(func() {
		if rustBackendEnabled {
			log.Infof("service discovery: Rust backend enabled (DD_SERVICE_DISCOVERY_USE_RUST_BACKEND=true)")
		} else {
			log.Infof("service discovery: using Go backend (set DD_SERVICE_DISCOVERY_USE_RUST_BACKEND=true to use Rust)")
		}
	})
	if rustBackendEnabled {
		return s.getServicesRust(params)
	}
	return s.getServicesGo(params)
}
