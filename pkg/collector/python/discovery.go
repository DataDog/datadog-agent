// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	if pkgconfigsetup.Datadog().GetBool("python_lazy_loading") {
		pythonOnce.Do(func() {
			InitPython(common.GetPythonPaths()...)
		})
	}

	if rtloader == nil {
		return nil, errors.New("python is not initialized")
	}

	if service.Ports == nil {
		service.Ports = []DiscoveryPort{}
	}
	serviceJSON, err := json.Marshal(service)
	if err != nil {
		return nil, fmt.Errorf("could not marshal discovery service for python check %s: %w", integrationName, err)
	}

	glock, err := newStickyLock()
	if err != nil {
		return nil, err
	}
	defer glock.unlock()

	if !pkgconfigsetup.Datadog().GetBool("win_skip_com_init") {
		log.Debugf("Performing platform loading prep")
		err = platformLoaderPrep()
		if err != nil {
			return nil, err
		}
		defer platformLoaderDone() //nolint:errcheck
	} else {
		log.Infof("Skipping platform loading prep")
	}

	checkModule, checkClass, err := loadPythonCheckClass(integrationName)
	if err != nil {
		return nil, err
	}
	defer C.rtloader_decref(rtloader, checkClass)
	defer C.rtloader_decref(rtloader, checkModule)

	cServiceJSON := TrackedCString(string(serviceJSON))
	defer C.call_free(unsafe.Pointer(cServiceJSON))

	discoveryResult := C.discover_config(rtloader, checkClass, cServiceJSON)
	if discoveryResult == nil {
		if err := getRtLoaderError(); err != nil {
			return nil, fmt.Errorf("could not discover configs for python check %s: %w", integrationName, err)
		}
		return nil, fmt.Errorf("could not discover configs for python check %s", integrationName)
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(discoveryResult))

	return parseDiscoveryResult(integrationName, C.GoString(discoveryResult))
}

func loadPythonCheckClass(moduleName string) (*C.rtloader_pyobject_t, *C.rtloader_pyobject_t, error) {
	modules := []string{fmt.Sprintf("%s.%s", wheelNamespace, moduleName), moduleName}
	var checkModule *C.rtloader_pyobject_t
	var checkClass *C.rtloader_pyobject_t
	var loadErrors []string

	for _, name := range modules {
		cModuleName := TrackedCString(name)
		defer C.call_free(unsafe.Pointer(cModuleName))

		if res := C.get_class(rtloader, cModuleName, &checkModule, &checkClass); res != 0 {
			return checkModule, checkClass, nil
		}

		if err := getRtLoaderError(); err != nil {
			log.Debugf("Unable to load python module - %s: %v", name, err)
			loadErrors = append(loadErrors, fmt.Sprintf("unable to load python module %s: %v", name, err))
		} else {
			log.Debugf("Unable to load python module - %s", name)
			loadErrors = append(loadErrors, "unable to load python module "+name)
		}
	}

	errMsg := strings.Join(loadErrors, ", ")
	log.Debugf("Unable to load check %s: %s", moduleName, errMsg)
	return nil, nil, fmt.Errorf("unable to load check %s: %s", moduleName, errMsg)
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
