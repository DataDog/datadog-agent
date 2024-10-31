// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package file

import (
	"errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"

	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"

	"github.com/rapdev-io/datadog-secret-backend/secret"
)

type FileYamlBackendConfig struct {
	BackendType string `mapstructure:"backend_type"`
	FilePath    string `mapstructure:"file_path"`
}

type FileYamlBackend struct {
	BackendId string
	Config    FileYamlBackendConfig
	Secret    map[string]string
}

func NewFileYamlBackend(backendId string, bc map[string]interface{}) (
	*FileYamlBackend, error) {

	backendConfig := FileYamlBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to map backend configuration")
		return nil, err
	}

	content, err := ioutil.ReadFile(backendConfig.FilePath)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Str("file_path", backendConfig.FilePath).
			Msg("failed to read yaml secret file")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if err := yaml.Unmarshal(content, secretValue); err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Str("file_path", backendConfig.FilePath).
			Msg("failed to unmarshal yaml secret")
		return nil, err
	}

	backend := &FileYamlBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *FileYamlBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.Error().
		Str("backend_id", b.BackendId).
		Str("backend_type", b.Config.BackendType).
		Str("file_path", b.Config.FilePath).
		Str("secret_key", secretKey).
		Msg("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
