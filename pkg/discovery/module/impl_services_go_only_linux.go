// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Go-only getServices entry point, used when the binary is compiled without
// the dd_discovery_rust build tag or without cgo. In this configuration only
// the pure-Go implementation is available, so getServices always forwards to
// getServicesGo (defined in impl_linux.go).

//go:build linux && (!dd_discovery_rust || !cgo)

package module

import (
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// getServices acquires the module lock and delegates to the pure-Go backend.
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.getServicesGo(params)
}
