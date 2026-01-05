// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package file allows to fetch secrets from JSON and YAML files
package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-secret-backend/secret"
)

// JSONBackendConfig is the configuration for a JSON backend
type JSONBackendConfig struct {
	FilePath        string `mapstructure:"file_path"`
	MaxFileReadSize int64  `mapstructure:"max_file_read_size"`
}

// JSONBackend represents backend for JSON file
type JSONBackend struct {
	Config JSONBackendConfig
	Secret map[string]string
}

// NewJSONBackend returns a new JSON backend
func NewJSONBackend(bc map[string]interface{}) (
	*JSONBackend, error) {

	backendConfig := JSONBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	if backendConfig.MaxFileReadSize <= 0 {
		backendConfig.MaxFileReadSize = secret.DefaultMaxFileReadSize
	}

	if info, err := os.Stat(backendConfig.FilePath); err == nil && info.Size() > backendConfig.MaxFileReadSize {
		return nil, fmt.Errorf("secret file '%s' exceeds maximum size limit of %d bytes (actual: %d bytes)", backendConfig.FilePath, backendConfig.MaxFileReadSize, info.Size())
	}

	content, err := os.ReadFile(backendConfig.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read json secret file '%s': %s", backendConfig.FilePath, err)
	}

	secretValue := make(map[string]string, 0)
	if err := json.Unmarshal(content, &secretValue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json secret '%s': %s", backendConfig.FilePath, err)
	}

	backend := &JSONBackend{
		Config: backendConfig,
		Secret: secretValue,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *JSONBackend) GetSecretOutput(_ context.Context, secretKey string) secret.Output {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.Output{Value: &val, Error: nil}
	}
	es := secret.ErrKeyNotFound.Error()
	return secret.Output{Value: nil, Error: &es}
}
