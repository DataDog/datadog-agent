// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package env allows to fetch secrets from environment variables
package env

import (
	"context"
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

// EnvBackendConfig is the configuration for an environment variable backend
type EnvBackendConfig struct {
	// AllowedKeys optionally restricts which environment variables can be
	// resolved. When empty, any environment variable is accessible.
	AllowedKeys []string `mapstructure:"allowed_keys"`
}

// EnvBackend represents backend for environment variables
type EnvBackend struct {
	Config  EnvBackendConfig
	allowed map[string]struct{}
}

// NewEnvBackend returns a new environment variable backend
func NewEnvBackend(bc map[string]interface{}) (*EnvBackend, error) {
	backendConfig := EnvBackendConfig{}
	if err := mapstructure.Decode(bc, &backendConfig); err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	var allowed map[string]struct{}
	if len(backendConfig.AllowedKeys) > 0 {
		allowed = make(map[string]struct{}, len(backendConfig.AllowedKeys))
		for _, k := range backendConfig.AllowedKeys {
			allowed[k] = struct{}{}
		}
	}

	return &EnvBackend{Config: backendConfig, allowed: allowed}, nil
}

// GetSecretOutput retrieves a secret from an environment variable
func (b *EnvBackend) GetSecretOutput(_ context.Context, secretKey string) secret.Output {
	if b.allowed != nil {
		if _, ok := b.allowed[secretKey]; !ok {
			es := fmt.Sprintf("environment variable '%s' is not in allowed_keys", secretKey)
			return secret.Output{Value: nil, Error: &es}
		}
	}

	value := os.Getenv(secretKey)
	if value == "" {
		es := secret.ErrKeyNotFound.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: &value, Error: nil}
}
