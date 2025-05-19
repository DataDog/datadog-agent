// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package storage holds files related to storages for security profiles
package storage

import (
	"bytes"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// ActivityDumpStorage defines the interface implemented by all activity dump storages
type ActivityDumpStorage interface {
	// GetStorageType returns the storage type
	GetStorageType() config.StorageType
	// Persist saves the provided buffer to the persistent storage
	Persist(request config.StorageRequest, p *profile.Profile, raw *bytes.Buffer) error
	// SendTelemetry sends metrics using the provided metrics sender
	SendTelemetry(sender statsd.ClientInterface)
}
