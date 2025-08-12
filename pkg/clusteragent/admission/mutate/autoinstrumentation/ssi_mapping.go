// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

var (
	mu      sync.RWMutex
	mapping map[string]string // tag -> sha
)

func UpdateMapping(update map[string]state.RawConfig) {
	mu.Lock()
	defer mu.Unlock()
	for _, raw := range update {
		var m map[string]string
		if err := json.Unmarshal(raw.Config, &m); err == nil {
			mapping = m
		}
	}
}

func GetMapping() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]string, len(mapping))
	for k, v := range mapping {
		out[k] = v
	}
	return out
}
