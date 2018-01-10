// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// LoaderErrors is just an alias for a loader->error map
type LoaderErrors map[string]string

// loaderErrorStats holds the error objects
type acErrorStats struct {
	config map[string]string       // config file name -> error
	loader map[string]LoaderErrors // check name -> LoaderErrors
	run    map[check.ID]string     // check ID -> error
	m      sync.RWMutex
}

// newAcErrorStats returns an instance holding autoconfig errors stats
func newAcErrorStats() *acErrorStats {
	return &acErrorStats{
		config: make(map[string]string),
		loader: make(map[string]LoaderErrors),
		run:    make(map[check.ID]string),
	}
}

// setConfigError will safely set the error for a check configuration file
func (es *acErrorStats) setConfigError(checkName string, err string) {
	es.m.Lock()
	defer es.m.Unlock()

	es.config[checkName] = err
}

// removeConfigErrors removes the errors for a check config file
func (es *acErrorStats) removeConfigError(checkName string) {
	es.m.Lock()
	defer es.m.Unlock()

	delete(es.config, checkName)
}

// getConfigErrors will safely get the errors a check config file
func (es *acErrorStats) getConfigErrors() map[string]string {
	es.m.RLock()
	defer es.m.RUnlock()

	configCopy := make(map[string]string)
	for k, v := range es.config {
		configCopy[k] = v
	}

	return configCopy
}

// setLoaderError will safely set the error for that check and loader to the LoaderErrorStats
func (es *acErrorStats) setLoaderError(checkName string, loaderName string, err string) {
	es.m.Lock()
	defer es.m.Unlock()

	_, found := es.loader[checkName]
	if !found {
		es.loader[checkName] = make(LoaderErrors)
	}

	es.loader[checkName][loaderName] = err
}

// removeLoaderErrors removes the errors for a check (usually when successfully loaded)
func (es *acErrorStats) removeLoaderErrors(checkName string) {
	es.m.Lock()
	defer es.m.Unlock()

	delete(es.loader, checkName)
}

// getLoaderErrors will safely get the errors regarding loaders
func (es *acErrorStats) getLoaderErrors() map[string]LoaderErrors {
	es.m.RLock()
	defer es.m.RUnlock()

	errorsCopy := make(map[string]LoaderErrors)

	for check, loaderErrors := range es.loader {
		errorsCopy[check] = make(LoaderErrors)
		for loader, loaderError := range loaderErrors {
			errorsCopy[check][loader] = loaderError
		}
	}

	return errorsCopy
}

func (es *acErrorStats) setRunError(checkID check.ID, err string) {
	es.m.Lock()
	defer es.m.Unlock()

	es.run[checkID] = err
}

func (es *acErrorStats) removeRunError(checkID check.ID) {
	es.m.Lock()
	defer es.m.Unlock()

	delete(es.run, checkID)
}

func (es *acErrorStats) getRunErrors() map[check.ID]string {
	es.m.RLock()
	defer es.m.RUnlock()

	runCopy := make(map[check.ID]string)
	for k, v := range es.run {
		runCopy[k] = v
	}

	return runCopy
}
