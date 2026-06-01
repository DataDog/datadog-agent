// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

static inline void call_free(void* ptr) {
    _free(ptr);
}
*/
import "C"

// DiscoverConfig calls a Python integration's discovery bridge for a service and
// returns the discovered instance configs.
func DiscoverConfig(integrationName string, service DiscoveryService) ([]integration.Data, error) {
	if err := ensurePythonRuntime(); err != nil {
		return nil, err
	}

	if service.Ports == nil {
		service.Ports = []DiscoveryPort{}
	}
	serviceJSON, err := json.Marshal(service)
	if err != nil {
		return nil, fmt.Errorf("could not marshal discovery service for python check %s: %w", integrationName, err)
	}

	cleanup, err := preparePythonLoaderRuntime()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	loadedClass, err := loadPythonCheckClass(integrationName)
	if err != nil {
		return nil, err
	}
	defer loadedClass.decref()

	cServiceJSON := TrackedCString(string(serviceJSON))
	defer C.call_free(unsafe.Pointer(cServiceJSON))

	discoveryResult := C.discover_config(rtloader, loadedClass.class, cServiceJSON)
	if discoveryResult == nil {
		if err := getRtLoaderError(); err != nil {
			return nil, fmt.Errorf("could not discover configs for python check %s: %w", integrationName, err)
		}
		return nil, fmt.Errorf("could not discover configs for python check %s", integrationName)
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(discoveryResult))

	return parseDiscoveryResult(integrationName, C.GoString(discoveryResult))
}

func parseDiscoveryResult(integrationName string, resultJSON string) ([]integration.Data, error) {
	var rawConfigs []json.RawMessage
	if err := json.Unmarshal([]byte(resultJSON), &rawConfigs); err != nil {
		return nil, fmt.Errorf("could not parse discovered configs for python check %s: %w", integrationName, err)
	}

	if len(rawConfigs) == 0 {
		return nil, nil
	}

	configs := make([]integration.Data, 0, len(rawConfigs))
	for _, rawConfig := range rawConfigs {
		configs = append(configs, integration.Data(rawConfig))
	}

	return configs, nil
}
