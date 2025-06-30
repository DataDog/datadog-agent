// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
)

// FileConfigProvider collect configuration files from disk
type FileConfigProvider struct {
	Errors         map[string]string
	telemetryStore *telemetry.Store
}

// NewFileConfigProvider creates a new FileConfigProvider.
func NewFileConfigProvider(telemetryStore *telemetry.Store) *FileConfigProvider {
	return &FileConfigProvider{
		Errors:         make(map[string]string),
		telemetryStore: telemetryStore,
	}
}

// Collect returns the check configurations defined in Yaml files.
// Configs with advanced AD identifiers are filtered-out. They're handled by other file-based config providers.
func (c *FileConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	configs, errors, err := ReadConfigFiles(WithoutAdvancedAD)
	if err != nil {
		return nil, err
	}

	c.Errors = errors
	if c.telemetryStore != nil {
		c.telemetryStore.Errors.Set(float64(len(errors)), names.File)
	}

	return configs, nil
}

// IsUpToDate is not implemented for the file Providers as the files are not meant to change very often.
func (c *FileConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	return false, nil
}

// String returns a string representation of the FileConfigProvider
func (c *FileConfigProvider) String() string {
	return names.File
}

// GetConfigErrors is not implemented for the FileConfigProvider
func (c *FileConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}
