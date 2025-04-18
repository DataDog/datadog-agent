// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package health implements the internal healthcheck
package health

import "time"

// SavedCatalogState is a struct that contains the ping frequency and the catalog
// for use by a Restore function
type SavedCatalogState struct {
	pingFrequency time.Duration
	catalog       *catalog
}

// SetTestReadinessAndLivenessCatalog resets the readiness and liveness catalog with a new ping frequency
// Note that pingFrequency is a global variable, so multiple calls to this or similar functions are order-dependent
// when restoring the catalog
// Should be used in conjunction with RestoreReadinessAndLivenessCatalog
func SetTestReadinessAndLivenessCatalog(newPingFrequency time.Duration) SavedCatalogState {
	savedCatalog := SavedCatalogState{
		pingFrequency: pingFrequency,
		catalog:       readinessAndLivenessCatalog,
	}
	pingFrequency = newPingFrequency
	readinessAndLivenessCatalog = newCatalog()
	return savedCatalog
}

// RestoreReadinessAndLivenessCatalog restores the readiness and liveness catalog from a previous reset
func RestoreReadinessAndLivenessCatalog(savedCatalog SavedCatalogState) {
	for handle := range readinessAndLivenessCatalog.components {
		_ = Deregister(handle)
	}
	readinessAndLivenessCatalog = savedCatalog.catalog
	pingFrequency = savedCatalog.pingFrequency
}
