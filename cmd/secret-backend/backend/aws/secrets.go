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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/mitchellh/mapstructure"

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
	ForceString bool                 `mapstructure:"force_string"`
}

// SecretsManagerBackend represents backend for AWS Secret Manager
type SecretsManagerBackend struct {
	Config SecretsManagerBackendConfig
	Client secretsManagerClient
}

// NewSecretsManagerBackend returns a new AWS Secret Manager backend
func NewSecretsManagerBackend(bc map[string]interface{}) (
	*SecretsManagerBackend, error) {

	backendConfig := SecretsManagerBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	cfg, err := newConfigFromBackendConfig(backendConfig.Session)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize aws session: %s", err)
	}
	client := getSecretsManagerClient(*cfg)

	backend := &SecretsManagerBackend{
		Config: backendConfig,
		Client: client,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *SecretsManagerBackend) GetSecretOutput(secretString string) secret.Output {
	segments := strings.SplitN(secretString, ";", 2)
	if len(segments) != 2 {
		es := "invalid secret format, expected 'secret_id;key'"
		return secret.Output{Value: nil, Error: &es}
	}
	secretID := segments[0]
	secretKey := segments[1]

	input := &secretsmanager.GetSecretValueInput{
		SecretId: &secretID,
	}

	out, err := b.Client.GetSecretValue(context.TODO(), input)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	if out.SecretString == nil {
		es := "secret string is nil"
		return secret.Output{Value: nil, Error: &es}
	}

	var secretValue string
	if b.Config.ForceString {
		secretValue = *out.SecretString
	} else {
		// Try to parse as JSON first
		var jsonSecrets map[string]string
		if err := json.Unmarshal([]byte(*out.SecretString), &jsonSecrets); err != nil {
			// If JSON parsing fails, treat the entire string as the value
			secretValue = *out.SecretString
		} else {
			// If JSON parsing succeeds, look for the specific key
			if val, ok := jsonSecrets[secretKey]; ok {
				secretValue = val
			} else {
				es := secret.ErrKeyNotFound.Error()
				return secret.Output{Value: nil, Error: &es}
			}
		}
	}

	return secret.Output{Value: &secretValue, Error: nil}
}
