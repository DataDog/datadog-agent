// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
)

// DatadogMetricsStore stores DatadogMetric with `Name` as a key
type DatadogMetricsInternalStore struct {
	store map[string]model.DatadogMetricInternal
	lock  sync.RWMutex
}

// NewDatadogMetricsInternalStore creates a new NewDatadogMetricsInternalStore
func NewDatadogMetricsInternalStore() DatadogMetricsInternalStore {
	return DatadogMetricsInternalStore{
		store: make(map[string]model.DatadogMetricInternal),
	}
}

// Get returns `DatadogMetricInternal` for given id, returns nil if absent
func (ds *DatadogMetricsInternalStore) Get(id string) *model.DatadogMetricInternal {
	ds.lock.RLock()
	defer ds.lock.RUnlock()

	res, ok := ds.store[id]
	if !ok {
		return nil
	}

	return &res
}

// GetAll returns a copy of all store values
func (ds *DatadogMetricsInternalStore) GetAll() []model.DatadogMetricInternal {
	return ds.GetFiltered(func(model.DatadogMetricInternal) bool { return true })
}

// GetAll returns a copy of all store values matched by the `filter` function
func (ds *DatadogMetricsInternalStore) GetFiltered(filter func(model.DatadogMetricInternal) bool) []model.DatadogMetricInternal {
	ds.lock.RLock()
	defer ds.lock.RUnlock()

	datadogMetrics := make([]model.DatadogMetricInternal, 0, len(ds.store))
	for _, datadogMetric := range ds.store {
		if filter(datadogMetric) {
			datadogMetrics = append(datadogMetrics, datadogMetric)
		}
	}

	return datadogMetrics
}

// Set `DatadogMetricInternal` for id
func (ds *DatadogMetricsInternalStore) Set(id string, datadogMetric model.DatadogMetricInternal) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	ds.store[id] = datadogMetric
}

// Delete `DatadogMetricInternal` corresponding to id if present
func (ds *DatadogMetricsInternalStore) Delete(id string) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	delete(ds.store, id)
}

func (ds *DatadogMetricsInternalStore) Count() int {
	ds.lock.RLock()
	defer ds.lock.RUnlock()

	return len(ds.store)
}

// LockRead allows to get an item and leave the store in a locked state to allow safe Read -> Operation -> Write sequences
// Still locks if the key does not exist as you may want to prevent a concurrent Write.
// It's not very efficient to lock the whole store but it's probably enough for our use case.
func (ds *DatadogMetricsInternalStore) LockRead(id string, lockOnMissing bool) *model.DatadogMetricInternal {
	ds.lock.Lock()

	res, ok := ds.store[id]
	if !ok {
		if !lockOnMissing {
			ds.lock.Unlock()
		}

		return nil
	}

	return &res
}

// UnlockSet set the new DatadogMetricInternal value and releases the lock (previously aqcuired by `LockRead`)
func (ds *DatadogMetricsInternalStore) UnlockSet(id string, datadogMetric model.DatadogMetricInternal) {
	ds.store[id] = datadogMetric

	ds.lock.Unlock()
}
