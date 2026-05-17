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

// ConfigStore implements persistent KV store for configurations for rollbacks
// whenever a config is retrieved, we will store agent-side along with the payload sent
// to intake to enable "rollbacks" without sending sensitive data (in configs) back and forth
type ConfigStore interface {
	Close(context.Context) error
	StoreConfig(deviceID string, configType types.ConfigType, rawConfig string) (string, error)
	GetConfig(configUUID string) (string, *types.ConfigMetadata, error)
	CheckDuplicate(deviceID string, configType types.ConfigType, rawHash string) (string, error)
}
