// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package file

import (
	"os"

	"gopkg.in/yaml.v2"

	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"

	"github.com/DataDog/datadog-secret-backend/secret"
)

// YamlBackendConfig is the configuration for a YAML backend
type YamlBackendConfig struct {
	BackendType string `mapstructure:"backend_type"`
	FilePath    string `mapstructure:"file_path"`
}

// YamlBackend represents backend for YAML file
type YamlBackend struct {
	BackendID string
	Config    YamlBackendConfig
	Secret    map[string]string
}

// NewYAMLBackend returns a new YAML backend
func NewYAMLBackend(backendID string, bc map[string]interface{}) (
	*YamlBackend, error) {

	backendConfig := YamlBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendID).
			Msg("failed to map backend configuration")
		return nil, err
	}

	content, err := os.ReadFile(backendConfig.FilePath)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendID).
			Str("file_path", backendConfig.FilePath).
			Msg("failed to read yaml secret file")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if err := yaml.Unmarshal(content, secretValue); err != nil {
		log.Error().Err(err).Str("backend_id", backendID).
			Str("file_path", backendConfig.FilePath).
			Msg("failed to unmarshal yaml secret")
		return nil, err
	}

	backend := &YamlBackend{
		BackendID: backendID,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *YamlBackend) GetSecretOutput(secretKey string) secret.Output {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.Output{Value: &val, Error: nil}
	}
	es := secret.ErrKeyNotFound.Error()

	log.Error().
		Str("backend_id", b.BackendID).
		Str("backend_type", b.Config.BackendType).
		Str("file_path", b.Config.FilePath).
		Str("secret_key", secretKey).
		Msg(es)
	return secret.Output{Value: nil, Error: &es}
}
