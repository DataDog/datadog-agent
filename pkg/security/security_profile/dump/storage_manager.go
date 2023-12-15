// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// ActivityDumpStorage defines the interface implemented by all activity dump storages
type ActivityDumpStorage interface {
	// GetStorageType returns the storage type
	GetStorageType() config.StorageType
	// Persist saves the provided buffer to the persistent storage
	Persist(request config.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error
	// SendTelemetry sends metrics using the provided metrics sender
	SendTelemetry(sender sender.Sender)
}

// ActivityDumpStorageManager is used to manage activity dump storages
type ActivityDumpStorageManager struct {
	statsdClient statsd.ClientInterface
	storages     map[config.StorageType]ActivityDumpStorage

	metricsSender sender.Sender
}

// NewSecurityAgentStorageManager returns a new instance of ActivityDumpStorageManager
func NewSecurityAgentStorageManager(senderManager sender.SenderManager) (*ActivityDumpStorageManager, error) {
	manager := &ActivityDumpStorageManager{
		storages: make(map[config.StorageType]ActivityDumpStorage),
	}

	sender, err := senderManager.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	manager.metricsSender = sender

	// create remote storage
	remote, err := NewActivityDumpRemoteStorage()
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate remote storage: %w", err)
	}
	manager.storages[remote.GetStorageType()] = remote

	return manager, nil
}

// NewSecurityAgentCommandStorageManager returns a new instance of ActivityDumpStorageManager
func NewSecurityAgentCommandStorageManager(cfg *config.Config) (*ActivityDumpStorageManager, error) {
	manager := &ActivityDumpStorageManager{
		storages: make(map[config.StorageType]ActivityDumpStorage),
	}

	storage, err := NewActivityDumpLocalStorage(cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate storage: %w", err)
	}
	manager.storages[storage.GetStorageType()] = storage

	// create remote storage
	remote, err := NewActivityDumpRemoteStorage()
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate remote storage: %w", err)
	}
	manager.storages[remote.GetStorageType()] = remote

	return manager, nil
}

// NewActivityDumpStorageManager returns a new instance of ActivityDumpStorageManager
func NewActivityDumpStorageManager(cfg *config.Config, statsdClient statsd.ClientInterface, handler ActivityDumpHandler, m *ActivityDumpManager) (*ActivityDumpStorageManager, error) {
	manager := &ActivityDumpStorageManager{
		storages:     make(map[config.StorageType]ActivityDumpStorage),
		statsdClient: statsdClient,
	}

	storage, err := NewActivityDumpLocalStorage(cfg, m)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate storage: %w", err)
	}
	manager.storages[storage.GetStorageType()] = storage

	storage, err = NewActivityDumpRemoteStorageForwarder(handler)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate storage: %w", err)
	}
	manager.storages[storage.GetStorageType()] = storage

	return manager, nil
}

// Persist saves the provided dump to the requested storages
func (manager *ActivityDumpStorageManager) Persist(ad *ActivityDump) error {

	for format := range ad.StorageRequests {
		// set serialization format metadata
		ad.Serialization = format.String()

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
func (manager *ActivityDumpStorageManager) PersistRaw(requests []config.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {
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
		if manager.statsdClient != nil {
			if size := len(raw.Bytes()); size > 0 {
				tags := []string{"format:" + request.Format.String(), "storage_type:" + request.Type.String(), fmt.Sprintf("compression:%v", request.Compression)}
				if err := manager.statsdClient.Count(metrics.MetricActivityDumpSizeInBytes, int64(size), tags, 1.0); err != nil {
					seclog.Warnf("couldn't send %s metric: %v", metrics.MetricActivityDumpSizeInBytes, err)
				}

				if err := manager.statsdClient.Count(metrics.MetricActivityDumpPersistedDumps, 1, tags, 1.0); err != nil {
					seclog.Warnf("couldn't send %s metric: %v", metrics.MetricActivityDumpPersistedDumps, err)
				}
			}
		}
	}
	return nil
}

// SendTelemetry send telemetry of all storages
func (manager *ActivityDumpStorageManager) SendTelemetry() {
	for _, storage := range manager.storages {
		storage.SendTelemetry(manager.metricsSender)
	}
}
