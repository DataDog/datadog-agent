// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package azure allows to fetch secrets from Azure keyvault service
package azure

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.1/keyvault"
	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
)

// KeyVaultBackendConfig contains the configuration to connect for Azure backend
type KeyVaultBackendConfig struct {
	Session     SessionBackendConfig `mapstructure:"azure_session"`
	BackendType string               `mapstructure:"backend_type"`
	ForceString bool                 `mapstructure:"force_string"`
	KeyVaultURL string               `mapstructure:"keyvaulturl"`
	SecretID    string               `mapstructure:"secret_id"`
}

// KeyVaultBackend is a backend to fetch secrets from Azure
type KeyVaultBackend struct {
	BackendID string
	Config    KeyVaultBackendConfig
	Secret    map[string]string
}

// NewKeyVaultBackend returns a new backend for Azure
func NewKeyVaultBackend(backendID string, bc map[string]interface{}) (*KeyVaultBackend, error) {
	backendConfig := KeyVaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.WithError(err).Error("failed to map backend configuration")
		return nil, err
	}

	cfg, err := NewConfigFromBackendConfig(backendConfig.Session)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id": backendID,
		}).WithError(err).Error("failed to initialize Azure session")
		return nil, err
	}
	client := keyvault.New()
	client.Authorizer = *cfg

	out, err := client.GetSecret(context.Background(), backendConfig.KeyVaultURL, backendConfig.SecretID, "")
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id":   backendID,
			"backend_type": backendConfig.BackendType,
			"secret_id":    backendConfig.SecretID,
			"keyvaulturl":  backendConfig.KeyVaultURL,
		}).WithError(err).Error("failed to retrieve secret value")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if backendConfig.ForceString {
		secretValue["_"] = *out.Value
	} else {
		if err := json.Unmarshal([]byte(*out.Value), &secretValue); err != nil {
			// assume: not json, store as single key -> string value
			secretValue["_"] = *out.Value
		}
	}

	backend := &KeyVaultBackend{
		BackendID: backendID,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *KeyVaultBackend) GetSecretOutput(secretKey string) secret.Output {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.Output{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.WithFields(log.Fields{
		"backend_id":   b.BackendID,
		"backend_type": b.Config.BackendType,
		"secret_id":    b.Config.SecretID,
		"keyvaulturl":  b.Config.KeyVaultURL,
		"secret_key":   secretKey,
	}).Error("backend does not provide secret key")
	return secret.Output{Value: nil, Error: &es}
}
