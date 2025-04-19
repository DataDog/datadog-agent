// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package storage holds files related to storages for security profiles
package storage

import (
	"bytes"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
)

// ActivityDumpRemoteStorage is a remote storage that forwards dumps to the backend
type ActivityDumpRemoteStorage struct {
	backend *backend.ActivityDumpBackendSender
}

// NewActivityDumpRemoteStorage returns a new instance of ActivityDumpRemoteStorage
func NewActivityDumpRemoteStorage() (ActivityDumpStorage, error) {
	b, err := backend.NewActivityDumpBackendSender()
	if err != nil {
		return nil, err
	}

	return &ActivityDumpRemoteStorage{
		backend: b,
	}, nil
}

// GetStorageType returns the storage type of the ActivityDumpLocalStorage
func (storage *ActivityDumpRemoteStorage) GetStorageType() config.StorageType {
	return config.RemoteStorage
}

// Persist saves the provided buffer to the persistent storage
func (storage *ActivityDumpRemoteStorage) Persist(request config.StorageRequest, p *profile.Profile, raw *bytes.Buffer) error {
	return storage.backend.Persist(request, p, raw)
}

// SendTelemetry sends telemetry for the current storage
func (storage *ActivityDumpRemoteStorage) SendTelemetry(sender statsd.ClientInterface) {
	storage.backend.SendTelemetry(sender)
}
