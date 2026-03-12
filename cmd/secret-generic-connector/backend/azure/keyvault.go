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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/azureauth"
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
	"github.com/mitchellh/mapstructure"
)

// keyvaultClient is an interface for fetching secrets from Azure Key Vault.
// Tests provide a mock implementation.
type keyvaultClient interface {
	GetSecret(ctx context.Context, secretID string, secretVersion string) (value *string, err error)
}

// getKeyvaultClient is a variable that holds the function to create a new keyvaultClient.
// It is overwritten in tests.
var getKeyvaultClient = func(keyVaultURL, clientID string) (keyvaultClient, error) {
	return &keyvaultHTTPClient{
		vaultURL: strings.TrimRight(keyVaultURL, "/"),
		cred:     azureauth.NewManagedIdentityTokenProvider(clientID),
	}, nil
}

// KeyVaultBackendConfig contains the configuration to connect for Azure backend
type KeyVaultBackendConfig struct {
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

	client, err := getKeyvaultClient(backendConfig.KeyVaultURL, backendConfig.ClientID)
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
func (b *KeyVaultBackend) GetSecretOutput(ctx context.Context, secretName string) secret.Output {
	var secretID, secretKey string

	sections := strings.SplitN(secretName, ";", 2)
	if len(sections) == 1 {
		secretID = sections[0]
	} else {
		secretID = sections[0]
		secretKey = sections[1]
	}

	version := ""
	value, err := b.Client.GetSecret(ctx, secretID, version)
	if err != nil {
		return b.makeErrorResponse(err)
	}

	// no semi-colon, return the secret value as a flat string
	if secretKey == "" {
		return secret.Output{Value: value, Error: nil}
	}

	// secret value is treated as structured json
	secretValue := make(map[string]string, 0)
	err = json.Unmarshal([]byte(*value), &secretValue)
	if err == nil {
		if val, ok := secretValue[secretKey]; ok {
			return secret.Output{Value: &val, Error: nil}
		}
	}

	// See https://github.com/Azure/azure-sdk-for-net/issues/39434, Azure KeyVault can return an escaped string value
	// that is not parsable as is. We need to unquote it first.
	unquoted, err := strconv.Unquote(fmt.Sprintf(`"%s"`, *value))
	if err == nil {
		err = json.Unmarshal([]byte(unquoted), &secretValue)
		if err == nil {
			if val, ok := secretValue[secretKey]; ok {
				return secret.Output{Value: &val, Error: nil}
			}
		}
	}

	return b.makeErrorResponse(errors.New("value does not contain secret key"))
}

func (b *KeyVaultBackend) makeErrorResponse(err error) secret.Output {
	es := err.Error()
	return secret.Output{Value: nil, Error: &es}
}

// --- Raw HTTP implementation of Azure Key Vault (replaces azsecrets SDK) ---

const kvAPIVersion = "7.4"

type keyvaultHTTPClient struct {
	vaultURL string
	cred     azureauth.TokenProvider
}

// kvSecretBundle is the response from the Key Vault GetSecret API.
type kvSecretBundle struct {
	Value *string `json:"value"`
	ID    *string `json:"id"`
}

func (c *keyvaultHTTPClient) GetSecret(ctx context.Context, secretName, secretVersion string) (*string, error) {
	tokenResp, err := c.cred.GetToken(ctx, "https://vault.azure.net")
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure access token: %w", err)
	}

	secretURL := fmt.Sprintf("%s/secrets/%s", c.vaultURL, url.PathEscape(secretName))
	if secretVersion != "" {
		secretURL += "/" + url.PathEscape(secretVersion)
	}
	secretURL += "?api-version=" + kvAPIVersion

	req, err := http.NewRequestWithContext(ctx, "GET", secretURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Key Vault GetSecret returned %d: %s", resp.StatusCode, string(body))
	}

	var result kvSecretBundle
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Key Vault response: %w", err)
	}

	return result.Value, nil
}
