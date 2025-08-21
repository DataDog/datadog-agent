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
	"os"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/mitchellh/mapstructure"
)

// keyvaultClient is an interface that defines the methods we use from the ssm client
// As the AWS SDK doesn't provide a real mock, we'll have to make our own that
// matches this interface
type keyvaultClient interface {
	GetSecret(ctx context.Context, secretID string, secretVersion string, opt *azsecrets.GetSecretOptions) (result azsecrets.GetSecretResponse, err error)
}

// getKeyvaultClient is a variable that holds the function to create a new keyvaultClient
// it will be overwritten in tests
var getKeyvaultClient = func(keyVaultURL, clientID string) (keyvaultClient, error) {
	var err error
	var cred azcore.TokenCredential
	if clientID == "" {
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	} else {
		opts := azidentity.ManagedIdentityCredentialOptions{ID: azidentity.ClientID(clientID)}
		cred, err = azidentity.NewManagedIdentityCredential(&opts)
	}
	if err != nil && clientID == "" {
		return nil, fmt.Errorf("clientID not provided, could not get credentials: %s", err)
	} else if err != nil {
		return nil, fmt.Errorf("getting identity credentials: %s", err)
	}

	client, err := azsecrets.NewClient(keyVaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}
	return client, nil
}

// KeyVaultBackendConfig contains the configuration to connect for Azure backend
type KeyVaultBackendConfig struct {
	BackendType string `mapstructure:"backend_type"`
	KeyVaultURL string `mapstructure:"keyvaulturl"`
	ClientID    string `mapstructure:"clientid"`
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
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	// Get clientID of the identity in this order:
	// 1. clientID sent over stdin
	// 2. from the env var AZURE_CLIENT_ID
	// If neither is provided, we'll use the DefaultAzureCredential which sometimes
	// picks up the Managed Identity attached to the running VM
	clientID := backendConfig.ClientID
	if clientID == "" {
		if env, found := os.LookupEnv("AZURE_CLIENT_ID"); found {
			clientID = env
		}
	}

	client, err := getKeyvaultClient(backendConfig.KeyVaultURL, clientID)
	if err != nil {
		return nil, err
	}

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

	return b.makeErrorResponse(fmt.Errorf("value does not contain secret key"))
}

func (b *KeyVaultBackend) makeErrorResponse(err error) secret.Output {
	es := err.Error()
	return secret.Output{Value: nil, Error: &es}
}
