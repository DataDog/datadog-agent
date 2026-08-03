// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package store is provides persistent local storage for network device configurations (for NCM)
// utilizing bbolt - enabling rollback of configs w/o sending data to the Datadog backend
package store

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// ConfigNotFoundError indicates that the given UUID wasn't present in the store.
type ConfigNotFoundError struct {
	UUID string
}

// Type implements [types.RollbackError].
func (u *ConfigNotFoundError) Type() types.ErrorType {
	return types.ErrConfigNotPresent
}

func (u *ConfigNotFoundError) Error() string {
	return "raw config not found for UUID: " + u.UUID
}

var _ types.RollbackError = (*ConfigNotFoundError)(nil)

// ConfigStore implements persistent KV store for configurations for rollbacks
// whenever a config is retrieved, we will store agent-side along with the payload sent
// to intake to enable "rollbacks" without sending sensitive data (in configs) back and forth
type ConfigStore interface {
	Close(context.Context) error
	StoreConfig(deviceID string, configType types.ConfigType, rawConfig string) (configUUID string, configHash string, stored bool, err error)
	// GetConfig fetches data for a given UUID. It should return an
	// UnknownUUIDError if the UUID isn't present in the store (any other error
	// type indicates a problem with the store itself, such as data corruption)
	GetConfig(configUUID string) (string, *types.ConfigMetadata, error)
	CheckDuplicate(deviceID string, configType types.ConfigType, rawHash string) (string, error)
	GetAllConfigMetadata() ([]*types.ConfigMetadata, error)
}
