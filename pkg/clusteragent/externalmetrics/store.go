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

const (
	setOperation storeOperation = iota
	deleteOperation
)

type DatadogMetricInternalObserverFunc func(string, string)

// DatadogMetricInternalObserver allows to define functions to watch changes in Store
type DatadogMetricInternalObserver struct {
	SetFunc    DatadogMetricInternalObserverFunc
	DeleteFunc DatadogMetricInternalObserverFunc
}

// DatadogMetricsStore stores DatadogMetric with `Name` as a key
type DatadogMetricsInternalStore struct {
	store         map[string]model.DatadogMetricInternal
	lock          sync.RWMutex
	observers     map[storeOperation][]DatadogMetricInternalObserverFunc
	observersLock sync.RWMutex
}

type storeOperation int

// NewDatadogMetricsInternalStore creates a new NewDatadogMetricsInternalStore
func NewDatadogMetricsInternalStore() DatadogMetricsInternalStore {
	return DatadogMetricsInternalStore{
		store: make(map[string]model.DatadogMetricInternal),
		observers: map[storeOperation][]DatadogMetricInternalObserverFunc{
			setOperation:    make([]DatadogMetricInternalObserverFunc, 0),
			deleteOperation: make([]DatadogMetricInternalObserverFunc, 0),
		},
	}
}

// RegisterObserver registers an observer that will be notified when changes happen in the store
// Current implementation does not scale beyond a handful of observers.
// Calls are made synchronously to each observer for each operation.
// The store guarantees that any lock has been released before calling observers.
func (ds *DatadogMetricsInternalStore) RegisterObserver(observer DatadogMetricInternalObserver) {
	ds.observersLock.Lock()
	defer ds.observersLock.Unlock()

	addObserver := func(operationType storeOperation, observerFunc DatadogMetricInternalObserverFunc) {
		if observerFunc != nil {
			ds.observers[operationType] = append(ds.observers[operationType], observerFunc)
		}
	}

	addObserver(setOperation, observer.SetFunc)
	addObserver(deleteOperation, observer.DeleteFunc)
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

// Count returns number of elements in store
func (ds *DatadogMetricsInternalStore) Count() int {
	ds.lock.RLock()
	defer ds.lock.RUnlock()

	return len(ds.store)
}

// Set `DatadogMetricInternal` for id
func (ds *DatadogMetricsInternalStore) Set(id string, datadogMetric model.DatadogMetricInternal, sender string) {
	ds.lock.Lock()
	ds.store[id] = datadogMetric
	ds.lock.Unlock()

	ds.notify(setOperation, id, sender)
}

// Delete `DatadogMetricInternal` corresponding to id if present
func (ds *DatadogMetricsInternalStore) Delete(id, sender string) {
	ds.lock.Lock()
	_, exists := ds.store[id]
	delete(ds.store, id)
	ds.lock.Unlock()

	if exists {
		ds.notify(deleteOperation, id, sender)
	}
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

// Unlock allows to unlock after a read that do not require any modification to the internal object
func (ds *DatadogMetricsInternalStore) Unlock(id string) {
	ds.lock.Unlock()
}

// UnlockSet sets the new DatadogMetricInternal value and releases the lock (previously acquired by `LockRead`)
func (ds *DatadogMetricsInternalStore) UnlockSet(id string, datadogMetric model.DatadogMetricInternal, sender string) {
	ds.store[id] = datadogMetric
	ds.lock.Unlock()

	ds.notify(setOperation, id, sender)
}

// UnlockDelete deletes a DatadogMetricInternal and releases the lock (previously acquired by `LockRead`)
func (ds *DatadogMetricsInternalStore) UnlockDelete(id, sender string) {
	_, exists := ds.store[id]

	delete(ds.store, id)
	ds.lock.Unlock()

	if exists {
		ds.notify(deleteOperation, id, sender)
	}
}

// It's a very simple implementation of a notify process, but it's enough in our case as we aim at only 1 or 2 observers
func (ds *DatadogMetricsInternalStore) notify(operationType storeOperation, key, sender string) {
	ds.observersLock.RLock()
	defer ds.observersLock.RUnlock()

	for _, observer := range ds.observers[operationType] {
		observer(key, sender)
	}
}
