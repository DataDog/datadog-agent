// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package autodiscovery

import "sync"

// LoaderErrors is just an alias for a loader->error map
type LoaderErrors map[string]string

// loaderErrorStats holds the error objects
type loaderErrorStats struct {
	errors map[string]LoaderErrors
	m      sync.RWMutex
}

// newLoaderErrorStats returns an instance holding loader errors stats
func newLoaderErrorStats() *loaderErrorStats {
	return &loaderErrorStats{
		errors: make(map[string]LoaderErrors),
	}
}

// setError will safely set the error for that check and loader to the LoaderErrorStats
func (les *loaderErrorStats) setError(checkName string, loaderName string, err string) {
	les.m.Lock()
	defer les.m.Unlock()

	_, found := les.errors[checkName]
	if !found {
		les.errors[checkName] = make(LoaderErrors)
	}

	les.errors[checkName][loaderName] = err
}

// removeCheckErrors removes the errors for a check (usually when successfully loaded)
func (les *loaderErrorStats) removeCheckErrors(checkName string) {
	les.m.Lock()
	defer les.m.Unlock()

	delete(les.errors, checkName)
}

// GetErrors will safely get the errors from a LoaderErrorStats object
func (les *loaderErrorStats) getErrors() map[string]LoaderErrors {
	les.m.RLock()
	defer les.m.RUnlock()

	errorsCopy := make(map[string]LoaderErrors)

	for check, loaderErrors := range les.errors {
		errorsCopy[check] = make(LoaderErrors)
		for loader, loaderError := range loaderErrors {
			errorsCopy[check][loader] = loaderError
		}
	}

	return errorsCopy
}
