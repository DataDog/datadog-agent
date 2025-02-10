// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"expvar"
	"sync"

	"github.com/mohae/deepcopy"
)

var (
	acErrors = expvar.NewMap("autoconfig")
)

func setupExpvarErrors(au *AutoConfig) {
	acErrors.Set("ConfigErrors", expvar.Func(func() interface{} {
		return au.errorStats.getConfigErrors()
	}))
	acErrors.Set("ResolveWarnings", expvar.Func(func() interface{} {
		return au.errorStats.getResolveWarnings()
	}))
}

// loaderErrorStats holds the error objects
type errorStats struct {
	config  map[string]string   // config file name -> error
	resolve map[string][]string // config file name -> errors
	m       sync.RWMutex
}

// newErrorStats returns an instance holding autoconfig errors stats
func newErrorStats() *errorStats {
	return &errorStats{
		config:  make(map[string]string),
		resolve: make(map[string][]string),
	}
}

// setConfigError will safely set the error for a check configuration file
func (es *errorStats) setConfigError(checkName string, err string) {
	es.m.Lock()
	defer es.m.Unlock()

	es.config[checkName] = err
}

// removeConfigErrors removes the errors for a check config file
func (es *errorStats) removeConfigError(checkName string) {
	es.m.Lock()
	defer es.m.Unlock()

	delete(es.config, checkName)
}

// getConfigErrors will safely get the errors a check config file
func (es *errorStats) getConfigErrors() map[string]string {
	es.m.RLock()
	defer es.m.RUnlock()

	configCopy := make(map[string]string)
	for k, v := range es.config {
		configCopy[k] = v
	}

	return configCopy
}

// setResolveWarning will safely set the error for a check configuration file
func (es *errorStats) setResolveWarning(checkName string, err string) {
	es.m.Lock()
	defer es.m.Unlock()

	es.resolve[checkName] = append(es.resolve[checkName], err)
}

// removeResolveWarnings removes the errors for a check config file
func (es *errorStats) removeResolveWarnings(checkName string) {
	es.m.Lock()
	defer es.m.Unlock()

	delete(es.resolve, checkName)
}

// getResolveWarnings will safely get the errors a check config file
func (es *errorStats) getResolveWarnings() map[string][]string {
	es.m.RLock()
	defer es.m.RUnlock()

	return deepcopy.Copy(es.resolve).(map[string][]string)
}
