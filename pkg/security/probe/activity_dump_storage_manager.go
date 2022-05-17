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
	"github.com/DataDog/datadog-agent/pkg/security/probe/activity_dump"
)

// ActivityDumpStorage defines the interface implemented by all activity dump storages
type ActivityDumpStorage interface {
	// GetStorageType returns the storage type
	GetStorageType() activity_dump.StorageType
	// Persist saves the provided buffer to the persistent storage
	Persist(request activity_dump.StorageRequest, dump *ActivityDump, raw *bytes.Buffer) error
}

// ActivityDumpStorageManager is used to manage activity dump storages
type ActivityDumpStorageManager struct {
	probe    *Probe
	storages map[activity_dump.StorageType]ActivityDumpStorage
}

// NewActivityDumpStorageManager returns a new instance of ActivityDumpStorageManager
func NewActivityDumpStorageManager(p *Probe) (*ActivityDumpStorageManager, error) {
	storageFactory := []func(p *Probe) (ActivityDumpStorage, error){
		NewActivityDumpLocalStorage,
	}

	manager := &ActivityDumpStorageManager{
		storages: make(map[activity_dump.StorageType]ActivityDumpStorage),
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

// Persist saves the provided dump according to its storage requests
func (manager *ActivityDumpStorageManager) Persist(dump *ActivityDump) error {

	for format, _ := range dump.StorageRequests {

		// encode the dump as the request format
		data, err := dump.Encode(format)
		if err != nil {
			seclog.Errorf("couldn't persist activity dump [%s]: %v", dump.GetSelectorStr(), err)
			continue
		}

		if err = manager.persistRaw(dump.StorageRequests[format], dump, data); err != nil {
			seclog.Errorf("couldn't persist activity dump [%s] in [%s]: %v", dump.GetSelectorStr(), format, err)
			continue
		}

	}
	return nil
}

func (manager *ActivityDumpStorageManager) persistRaw(requests []activity_dump.StorageRequest, dump *ActivityDump, raw *bytes.Buffer) error {
	for _, request := range requests {
		storage, ok := manager.storages[request.Type]
		if !ok || storage == nil {
			seclog.Errorf("couldn't persist [%s] in [%s] storage: unknown storage", dump.GetSelectorStr(), request.Type)
			continue
		}

		if err := storage.Persist(request, dump, raw); err != nil {
			seclog.Errorf("couldn't persist [%s] in [%s] storage: %v", dump.GetSelectorStr(), request.Type, err)
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
