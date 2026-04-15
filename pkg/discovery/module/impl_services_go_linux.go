// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (!dd_discovery_rust || !cgo) && linux

package module

import (
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// getServices delegates to the pure-Go implementation when the Rust backend
// is not compiled in.
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	return s.getServicesGo(params)
}
