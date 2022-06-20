// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"bytes"
	"fmt"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
)

// ActivityDumpStorage defines the interface implemented by all activity dump storages
type ActivityDumpStorage interface {
	// GetStorageType returns the storage type
	GetStorageType() dump.StorageType
	// Persist saves the provided buffer to the persistent storage
	Persist(request dump.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error
}

// ActivityDumpStorageManager is used to manage activity dump storages
type ActivityDumpStorageManager struct {
	probe    *Probe
	storages map[dump.StorageType]ActivityDumpStorage
}

// NewSecurityAgentStorageManager returns a new instance of ActivityDumpStorageManager
func NewSecurityAgentStorageManager() (*ActivityDumpStorageManager, error) {
	manager := &ActivityDumpStorageManager{
		storages: make(map[dump.StorageType]ActivityDumpStorage),
	}

	// create remote storage
	remote, err := NewActivityDumpRemoteStorage()
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate remote storage: %w", err)
	}
	manager.storages[remote.GetStorageType()] = remote

	return manager, nil
}

// NewActivityDumpStorageManager returns a new instance of ActivityDumpStorageManager
func NewActivityDumpStorageManager(p *Probe) (*ActivityDumpStorageManager, error) {
	storageFactory := []func(p *Probe) (ActivityDumpStorage, error){
		NewActivityDumpLocalStorage,
		NewActivityDumpRemoteStorageForwarder,
	}

	manager := &ActivityDumpStorageManager{
		storages: make(map[dump.StorageType]ActivityDumpStorage),
		probe:    p,
	}
	for _, factory := range storageFactory {
		storage, err := factory(p)
		if err != nil {
			return nil, fmt.Errorf("couldn't instantiate storage: %w", err)
		}
		manager.storages[storage.GetStorageType()] = storage
	}
	return manager, nil
}

// Persist saves the provided dump to the requested storages
func (manager *ActivityDumpStorageManager) Persist(ad *ActivityDump) error {

	for format := range ad.StorageRequests {

		// encode the dump as the request format
		data, err := ad.Encode(format)
		if err != nil {
			seclog.Errorf("couldn't persist activity dump [%s]: %v", ad.GetSelectorStr(), err)
			continue
		}

		if err = manager.PersistRaw(ad.StorageRequests[format], ad, data); err != nil {
			seclog.Errorf("couldn't persist activity dump [%s] in [%s]: %v", ad.GetSelectorStr(), format, err)
			continue
		}

	}
	return nil
}

// PersistRaw saves the provided dump to the requested storages
func (manager *ActivityDumpStorageManager) PersistRaw(requests []dump.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {
	for _, request := range requests {
		storage, ok := manager.storages[request.Type]
		if !ok || storage == nil {
			seclog.Errorf("couldn't persist [%s] in [%s] storage: unknown storage", ad.GetSelectorStr(), request.Type)
			continue
		}

		if err := storage.Persist(request, ad, raw); err != nil {
			seclog.Errorf("couldn't persist [%s] in [%s] storage: %v", ad.GetSelectorStr(), request.Type, err)
			continue
		}

		// send dump metric
		if manager.probe != nil {
			if size := len(raw.Bytes()); size > 0 {
				tags := []string{"format:" + request.Format.String(), "storage_type:" + request.Type.String(), fmt.Sprintf("compression:%v", request.Compression)}
				if err := manager.probe.statsdClient.Gauge(metrics.MetricActivityDumpSizeInBytes, float64(size), tags, 1.0); err != nil {
					seclog.Warnf("couldn't send %s metric: %v", metrics.MetricActivityDumpSizeInBytes, err)
				}
			}
		}
	}
	return nil
}
