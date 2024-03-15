// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"expvar"
	"sync"
)

var (
	acErrors   = expvar.NewMap("autoconfig")
	errorStats = newAcErrorStats()
)

func setupAcErrors() {
	acErrors.Set("ConfigErrors", expvar.Func(func() interface{} {
		return errorStats.getConfigErrors()
	}))
	acErrors.Set("ResolveWarnings", expvar.Func(func() interface{} {
		return errorStats.getResolveWarnings()
	}))
}

// loaderErrorStats holds the error objects
type acErrorStats struct {
	config  map[string]string   // config file name -> error
	resolve map[string][]string // config file name -> errors
	m       sync.RWMutex
}

// newAcErrorStats returns an instance holding autoconfig errors stats
func newAcErrorStats() *acErrorStats {
	return &acErrorStats{
		config:  make(map[string]string),
		resolve: make(map[string][]string),
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

// setResolveWarning will safely set the error for a check configuration file
func (es *acErrorStats) setResolveWarning(checkName string, err string) {
	es.m.Lock()
	defer es.m.Unlock()

	es.resolve[checkName] = append(es.resolve[checkName], err)
}

// removeResolveWarnings removes the errors for a check config file
func (es *acErrorStats) removeResolveWarnings(checkName string) {
	es.m.Lock()
	defer es.m.Unlock()

	delete(es.resolve, checkName)
}

// getResolveWarnings will safely get the errors a check config file
func (es *acErrorStats) getResolveWarnings() map[string][]string {
	es.m.RLock()
	defer es.m.RUnlock()

	resolveCopy := make(map[string][]string)
	for k, v := range es.resolve {
		resolveCopy[k] = v
	}

	return resolveCopy
}

// GetConfigErrors gets the config errors
func GetConfigErrors() map[string]string {
	return errorStats.getConfigErrors()
}

// GetResolveWarnings get the resolve warnings/errors
func GetResolveWarnings() map[string][]string {
	return errorStats.getResolveWarnings()
}
