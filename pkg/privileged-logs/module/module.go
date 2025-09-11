// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package module implements the privileged logs module for the system-probe.
package module

import (
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// NewPrivilegedLogsModule creates a new instance of the privileged logs module.
var NewPrivilegedLogsModule = func() module.Module {
	return &privilegedLogsModule{}
}

var _ module.Module = &privilegedLogsModule{}

type privilegedLogsModule struct{}

// GetStats returns stats for the module
func (f *privilegedLogsModule) GetStats() map[string]interface{} {
	return nil
}

// Register registers endpoints for the module to expose data
func (f *privilegedLogsModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/open", f.openFileHandler).Methods("POST")
	return nil
}

// Close cleans up the module
func (f *privilegedLogsModule) Close() {
	// No cleanup needed
}
