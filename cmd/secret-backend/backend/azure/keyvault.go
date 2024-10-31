// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package azure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.1/keyvault"
	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"
	log "github.com/sirupsen/logrus"
)

type AzureKeyVaultBackendConfig struct {
	AzureSession AzureSessionBackendConfig `mapstructure:"azure_session"`
	BackendType  string                    `mapstructure:"backend_type"`
	ForceString  bool                      `mapstructure:"force_string"`
	KeyVaultURL  string                    `mapstructure:"keyvaulturl"`
	SecretId     string                    `mapstructure:"secret_id"`
}

type AzureKeyVaultBackend struct {
	BackendId string
	Config    AzureKeyVaultBackendConfig
	Secret    map[string]string
}

func NewAzureKeyVaultBackend(backendId string, bc map[string]interface{}) (*AzureKeyVaultBackend, error) {
	backendConfig := AzureKeyVaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.WithError(err).Error("failed to map backend configuration")
		return nil, err
	}

	cfg, err := NewAzureConfigFromBackendConfig(backendId, backendConfig.AzureSession)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id": backendId,
		}).WithError(err).Error("failed to initialize Azure session")
		return nil, err
	}
	client := keyvault.New()
	client.Authorizer = *cfg

	out, err := client.GetSecret(context.Background(), backendConfig.KeyVaultURL, backendConfig.SecretId, "")
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id":   backendId,
			"backend_type": backendConfig.BackendType,
			"secret_id":    backendConfig.SecretId,
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

	backend := &AzureKeyVaultBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *AzureKeyVaultBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.WithFields(log.Fields{
		"backend_id":   b.BackendId,
		"backend_type": b.Config.BackendType,
		"secret_id":    b.Config.SecretId,
		"keyvaulturl":  b.Config.KeyVaultURL,
		"secret_key":   secretKey,
	}).Error("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
