// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"

	"github.com/DataDog/datadog-secret-backend/secret"
)

type AwsSecretsManagerBackendConfig struct {
	AwsSession  AwsSessionBackendConfig `mapstructure:"aws_session"`
	BackendType string                  `mapstructure:"backend_type"`
	ForceString bool                    `mapstructure:"force_string"`
	SecretId    string                  `mapstructure:"secret_id"`
}

type AwsSecretsManagerBackend struct {
	BackendId string
	Config    AwsSecretsManagerBackendConfig
	Secret    map[string]string
}

func NewAwsSecretsManagerBackend(backendId string, bc map[string]interface{}) (
	*AwsSecretsManagerBackend, error) {

	backendConfig := AwsSecretsManagerBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).
			Str("backend_id", backendId).
			Msg("failed to map backend configuration")
		return nil, err
	}

	cfg, err := NewAwsConfigFromBackendConfig(backendId, backendConfig.AwsSession)
	if err != nil {
		log.Error().Err(err).
			Str("backend_id", backendId).
			Msg("failed to initialize aws session")
		return nil, err
	}
	client := secretsmanager.NewFromConfig(*cfg)

	// GetSecretValue
	input := &secretsmanager.GetSecretValueInput{
		SecretId: &backendConfig.SecretId,
	}
	out, err := client.GetSecretValue(context.TODO(), input)
	if err != nil {
		log.Error().Err(err).
			Str("backend_id", backendId).
			Str("backend_type", backendConfig.BackendType).
			Str("secret_id", backendConfig.SecretId).
			Str("aws_access_key_id", backendConfig.AwsSession.AwsAccessKeyId).
			Str("aws_profile", backendConfig.AwsSession.AwsProfile).
			Msg("failed to retreive secret value")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if backendConfig.ForceString {
		secretValue["_"] = *out.SecretString
	} else {
		if err := json.Unmarshal([]byte(*out.SecretString), &secretValue); err != nil {
			secretValue["_"] = *out.SecretString
		}
	}

	backend := &AwsSecretsManagerBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *AwsSecretsManagerBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.Error().
		Str("backend_id", b.BackendId).
		Str("backend_type", b.Config.BackendType).
		Str("secret_id", b.Config.SecretId).
		Str("secret_key", secretKey).
		Msg("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
