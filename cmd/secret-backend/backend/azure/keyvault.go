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
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
)

// keyvaultClient is an interface that defines the methods we use from the ssm client
// As the AWS SDK doesn't provide a real mock, we'll have to make our own that
// matches this interface
type keyvaultClient interface {
	GetSecret(ctx context.Context, secretID string, secretVersion string, opt *azsecrets.GetSecretOptions) (result azsecrets.GetSecretResponse, err error)
}

// getKeyvaultClient is a variable that holds the function to create a new keyvaultClient
// it will be overwritten in tests
var getKeyvaultClient = func(keyVaultURL string) keyvaultClient {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Errorf("getting default credentials: %s", err)
	}
	client, err := azsecrets.NewClient(keyVaultURL, cred, nil)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	return client
}

// KeyVaultBackendConfig contains the configuration to connect for Azure backend
type KeyVaultBackendConfig struct {
	BackendType string `mapstructure:"backend_type"`
	KeyVaultURL string `mapstructure:"keyvaulturl"`
	SecretID    string `mapstructure:"secret_id"`
}

// KeyVaultBackend is a backend to fetch secrets from Azure
type KeyVaultBackend struct {
	Config KeyVaultBackendConfig
	Client keyvaultClient
}

// NewKeyVaultBackend returns a new backend for Azure
func NewKeyVaultBackend(bc map[string]interface{}) (*KeyVaultBackend, error) {
	backendConfig := KeyVaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.WithError(err).Error("failed to map backend configuration")
		return nil, err
	}

	client := getKeyvaultClient(backendConfig.KeyVaultURL)
	backend := &KeyVaultBackend{
		Config: backendConfig,
		Client: client,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *KeyVaultBackend) GetSecretOutput(secretName string) secret.Output {
	var secretID, secretKey string

	sections := strings.SplitN(secretName, ";", 2)
	if len(sections) == 1 {
		secretID = sections[0]
	} else {
		secretID = sections[0]
		secretKey = sections[1]
	}

	version := ""
	out, err := b.Client.GetSecret(context.Background(), secretID, version, nil)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_type": b.Config.BackendType,
			"secret_id":    secretID,
			"keyvaulturl":  b.Config.KeyVaultURL,
		}).WithError(err).Error("failed to retrieve secret value")
		return b.makeErrorResponse(err)
	}

	// no semi-colon, return the secret value as a flat string
	if secretKey == "" {
		return secret.Output{Value: out.Value, Error: nil}
	}

	// secret value is treated as structured json
	secretValue := make(map[string]string, 0)
	err = json.Unmarshal([]byte(*out.Value), &secretValue)
	if err == nil {
		if val, ok := secretValue[secretKey]; ok {
			return secret.Output{Value: &val, Error: nil}
		}
	}

	// See https://github.com/Azure/azure-sdk-for-net/issues/39434, Azure KeyVault can return an escaped string value
	// that is not parsable as is. We need to unquote it first.
	unquoted, err := strconv.Unquote(fmt.Sprintf(`"%s"`, *out.Value))
	if err == nil {
		err = json.Unmarshal([]byte(unquoted), &secretValue)
		if err == nil {
			if val, ok := secretValue[secretKey]; ok {
				return secret.Output{Value: &val, Error: nil}
			}
		}
	}

	log.WithFields(log.Fields{
		"backend_type": b.Config.BackendType,
		"secret_id":    secretID,
		"keyvaulturl":  b.Config.KeyVaultURL,
		"secret_key":   secretKey,
	}).Error("value does not contain secret key")

	return b.makeErrorResponse(fmt.Errorf("value does not contain secret key"))
}

func (b *KeyVaultBackend) makeErrorResponse(err error) secret.Output {
	es := err.Error()
	return secret.Output{Value: nil, Error: &es}
}
