// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package module implements the privileged logs module for the system-probe.
package module

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// NewPrivilegedLogsModule creates a new instance of the privileged logs module.
var NewPrivilegedLogsModule = func() module.Module {
	cache, err := simplelru.NewLRU[string, struct{}](128, nil)
	if err != nil {
		log.Errorf("Failed to create LRU cache for privileged logs module: %v", err)
		cache = nil
	}

	return &privilegedLogsModule{
		informedPaths: cache,
	}
}

var _ module.Module = &privilegedLogsModule{}

type privilegedLogsModule struct {
	informedPaths *simplelru.LRU[string, struct{}]
	mu            sync.RWMutex
}

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
