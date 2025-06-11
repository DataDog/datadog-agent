// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package azure

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.1/keyvault"
	"github.com/Azure/go-autorest/autorest"
	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/stretchr/testify/assert"
)

// keyvaultMockClient is the struct we'll use to mock the Azure KeyVault client
// for unit tests. E2E tests should be written with the real client.
type keyvaultMockClient struct {
	secrets map[string]interface{}
}

func (c *keyvaultMockClient) GetSecret(_ context.Context, _ string, secretName string, _ string) (result keyvault.SecretBundle, err error) {
	if _, ok := c.secrets[secretName]; ok {
		val := c.secrets[secretName].(string)
		return keyvault.SecretBundle{
			Value: &val,
			ID:    &secretName,
		}, nil
	}
	return keyvault.SecretBundle{}, secret.ErrKeyNotFound
}

func TestKeyvaultBackend(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]interface{}{
			"key1": "{\"user\":\"foo\",\"password\":\"bar\"}",
			"key2": "{\"foo\":\"bar\"}",
		},
	}
	getKeyvaultClient = func(_ *autorest.Authorizer) keyvaultClient {
		return mockClient
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"secret_id":    "key1",
		"force_string": false,
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend("keyvault-backend", keyvaultBackendParams)
	assert.NoError(t, err)

	// Top-level keys are not fetchable
	secretOutput := keyvaultSecretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	secretOutput = keyvaultSecretsBackend.GetSecretOutput("key2")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	// But the contents under the selected key are
	secretOutput = keyvaultSecretsBackend.GetSecretOutput("user")
	assert.Equal(t, "foo", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = keyvaultSecretsBackend.GetSecretOutput("password")
	assert.Equal(t, "bar", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}

func TestKeyvaultBackend_ForceString(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]interface{}{
			"key1": "{\"user\":\"foo\",\"password\":\"bar\"}",
			"key2": "{\"foo\":\"bar\"}",
		},
	}
	getKeyvaultClient = func(_ *autorest.Authorizer) keyvaultClient {
		return mockClient
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"secret_id":    "key1",
		"force_string": true,
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend("keyvault-backend", keyvaultBackendParams)
	assert.NoError(t, err)

	secretOutput := keyvaultSecretsBackend.GetSecretOutput("_")
	assert.Equal(t, "{\"user\":\"foo\",\"password\":\"bar\"}", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = keyvaultSecretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestKeyvaultBackend_NotJSON(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]interface{}{
			"key1": "not json",
			"key2": "foobar",
		},
	}
	getKeyvaultClient = func(_ *autorest.Authorizer) keyvaultClient {
		return mockClient
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"secret_id":    "key1",
		"force_string": false,
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend("keyvault-backend", keyvaultBackendParams)
	assert.NoError(t, err)

	// Top-level keys are not fetchable
	secretOutput := keyvaultSecretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	secretOutput = keyvaultSecretsBackend.GetSecretOutput("key2")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	// But the contents under the selected key are
	secretOutput = keyvaultSecretsBackend.GetSecretOutput("_")
	assert.Equal(t, "not json", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}

func TestKeyVaultBackend_issue39434(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]interface{}{
			"key1": "{\\\"foo\\\":\\\"bar\\\"}",
		},
	}
	getKeyvaultClient = func(_ *autorest.Authorizer) keyvaultClient {
		return mockClient
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"secret_id":    "key1",
		"force_string": false,
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend("keyvault-backend", keyvaultBackendParams)
	assert.NoError(t, err)

	// Top-level keys are not fetchable
	secretOutput := keyvaultSecretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	// But the contents under the selected key are
	secretOutput = keyvaultSecretsBackend.GetSecretOutput("foo")
	assert.Equal(t, "bar", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}
