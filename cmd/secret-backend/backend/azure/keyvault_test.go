// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package azure

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/stretchr/testify/assert"
)

// keyvaultMockClient is the struct we'll use to mock the Azure KeyVault client
// for unit tests. E2E tests should be written with the real client.
type keyvaultMockClient struct {
	secrets map[string]interface{}
}

func (c *keyvaultMockClient) GetSecret(_ context.Context, secretName string, _ string, _ *azsecrets.GetSecretOptions) (result azsecrets.GetSecretResponse, err error) {
	if _, ok := c.secrets[secretName]; ok {
		val := c.secrets[secretName].(string)
		secretID := azsecrets.ID(secretName)
		return azsecrets.GetSecretResponse{
			Secret: azsecrets.Secret{
				Value: &val,
				ID:    &secretID,
			},
		}, nil
	}
	return azsecrets.GetSecretResponse{}, secret.ErrKeyNotFound
}

func TestKeyvaultBackend(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]interface{}{
			"key1": "{\"user\":\"foo\",\"password\":\"bar\"}",
			"key2": "{\"foo\":\"bar\"}",
		},
	}
	getKeyvaultClient = func(_ string) (keyvaultClient, error) {
		return mockClient, nil
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"keyvaulturl":  "https://my-vault.vault.azure.com/",
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend(keyvaultBackendParams)
	assert.NoError(t, err)

	// Top-level key will be fetched as json
	secretOutput := keyvaultSecretsBackend.GetSecretOutput("key1")
	assert.Equal(t, "{\"user\":\"foo\",\"password\":\"bar\"}", *secretOutput.Value)

	// Index into secret json
	secretOutput = keyvaultSecretsBackend.GetSecretOutput("key1;user")
	assert.Equal(t, "foo", *secretOutput.Value)

	secretOutput = keyvaultSecretsBackend.GetSecretOutput("key3")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestKeyVaultBackend_issue39434(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]interface{}{
			"key1": "{\\\"foo\\\":\\\"bar\\\"}",
		},
	}
	getKeyvaultClient = func(_ string) (keyvaultClient, error) {
		return mockClient, nil
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend(keyvaultBackendParams)
	assert.NoError(t, err)

	// Top-level keys are not fetchable
	secretOutput := keyvaultSecretsBackend.GetSecretOutput("key1")
	assert.Equal(t, "{\\\"foo\\\":\\\"bar\\\"}", *secretOutput.Value)

	// Index into secret json
	secretOutput = keyvaultSecretsBackend.GetSecretOutput("key1;foo")
	assert.Equal(t, "bar", *secretOutput.Value)
}
