// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package aws allows to fetch secrets from Aws SSM and Secrets Manager service
package aws

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"

	"github.com/DataDog/datadog-secret-backend/secret"
)

// secretsManagerClient is an interface that defines the methods we use from the ssm client
// As the AWS SDK doesn't provide a real mock, we'll have to make our own that
// matches this interface
type secretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// getSecretsManagerClient is a variable that holds the function to create a new secretsManagerClient
// it will be overwritten in tests
var getSecretsManagerClient = func(cfg aws.Config) secretsManagerClient {
	return secretsmanager.NewFromConfig(cfg)
}

// SecretsManagerBackendConfig is the configuration for a AWS Secret Manager backend
type SecretsManagerBackendConfig struct {
	Session     SessionBackendConfig `mapstructure:"aws_session"`
	BackendType string               `mapstructure:"backend_type"`
	ForceString bool                 `mapstructure:"force_string"`
	SecretID    string               `mapstructure:"secret_id"`
}

// SecretsManagerBackend represents backend for AWS Secret Manager
type SecretsManagerBackend struct {
	BackendID string
	Config    SecretsManagerBackendConfig
	Secret    map[string]string
}

// NewSecretsManagerBackend returns a new AWS Secret Manager backend
func NewSecretsManagerBackend(backendID string, bc map[string]interface{}) (
	*SecretsManagerBackend, error) {

	backendConfig := SecretsManagerBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).
			Str("backend_id", backendID).
			Msg("failed to map backend configuration")
		return nil, err
	}

	cfg, err := NewConfigFromBackendConfig(backendConfig.Session)
	if err != nil {
		log.Error().Err(err).
			Str("backend_id", backendID).
			Msg("failed to initialize aws session")
		return nil, err
	}
	client := getSecretsManagerClient(*cfg)

	// GetSecretValue
	input := &secretsmanager.GetSecretValueInput{
		SecretId: &backendConfig.SecretID,
	}
	out, err := client.GetSecretValue(context.TODO(), input)
	if err != nil {
		log.Error().Err(err).
			Str("backend_id", backendID).
			Str("backend_type", backendConfig.BackendType).
			Str("secret_id", backendConfig.SecretID).
			Str("aws_access_key_id", backendConfig.Session.AccessKeyID).
			Str("aws_profile", backendConfig.Session.Profile).
			Msg("failed to retrieve secret value")
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

	backend := &SecretsManagerBackend{
		BackendID: backendID,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *SecretsManagerBackend) GetSecretOutput(secretKey string) secret.Output {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.Output{Value: &val, Error: nil}
	}
	es := secret.ErrKeyNotFound.Error()

	log.Error().
		Str("backend_id", b.BackendID).
		Str("backend_type", b.Config.BackendType).
		Str("secret_id", b.Config.SecretID).
		Str("secret_key", secretKey).
		Msg(es)
	return secret.Output{Value: nil, Error: &es}
}
