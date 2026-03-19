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
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
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
	getKeyvaultClient = func(_ KeyVaultBackendConfig) (keyvaultClient, error) {
		return mockClient, nil
	}

	// v0-style: azure_* nested under azure_session (sibling to keyvaulturl)
	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"keyvaulturl":  "https://my-vault.vault.azure.com/",
		"azure_session": map[string]interface{}{
			"azure_client_id": "123abc45-123a-abcd-123a-123abc456def",
		},
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend(keyvaultBackendParams)
	assert.NoError(t, err)

	ctx := context.Background()
	// Top-level key will be fetched as json
	secretOutput := keyvaultSecretsBackend.GetSecretOutput(ctx, "key1")
	assert.Equal(t, "{\"user\":\"foo\",\"password\":\"bar\"}", *secretOutput.Value)

	// Index into secret json
	secretOutput = keyvaultSecretsBackend.GetSecretOutput(ctx, "key1;user")
	assert.Equal(t, "foo", *secretOutput.Value)

	secretOutput = keyvaultSecretsBackend.GetSecretOutput(ctx, "key3")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestKeyVaultBackend_issue39434(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]interface{}{
			"key1": "{\\\"foo\\\":\\\"bar\\\"}",
		},
	}
	getKeyvaultClient = func(_ KeyVaultBackendConfig) (keyvaultClient, error) {
		return mockClient, nil
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"clientid":     "123abc45-123a-abcd-123a-123abc456def",
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend(keyvaultBackendParams)
	assert.NoError(t, err)

	ctx := context.Background()
	// Top-level keys are not fetchable
	secretOutput := keyvaultSecretsBackend.GetSecretOutput(ctx, "key1")
	assert.Equal(t, "{\\\"foo\\\":\\\"bar\\\"}", *secretOutput.Value)

	// Index into secret json
	secretOutput = keyvaultSecretsBackend.GetSecretOutput(ctx, "key1;foo")
	assert.Equal(t, "bar", *secretOutput.Value)
}

// TestKeyVaultBackend_clientSecretCredential verifies that tenant_id + client_id + client_secret
// are forwarded correctly to the keyvault client factory.
func TestKeyVaultBackend_clientSecretCredential(t *testing.T) {
	getKeyvaultClient = func(cfg KeyVaultBackendConfig) (keyvaultClient, error) {
		s := cfg.AzureSession
		assert.Equal(t, "my-tenant-id", s.AzureTenantID)
		assert.Equal(t, "my-client-id", s.AzureClientID)
		assert.Equal(t, "my-client-secret", s.AzureClientSecret)
		return &keyvaultMockClient{secrets: map[string]interface{}{}}, nil
	}
	bc := map[string]interface{}{
		"keyvaulturl": "https://my-vault.vault.azure.com/",
		"azure_session": map[string]interface{}{
			"azure_tenant_id":     "my-tenant-id",
			"azure_client_id":     "my-client-id",
			"azure_client_secret": "my-client-secret",
		},
	}
	_, err := NewKeyVaultBackend(bc)
	assert.NoError(t, err)
}

// TestKeyVaultBackend_clientCertificateCredential verifies that tenant_id + client_id + certificate_path
// are forwarded correctly to the keyvault client factory.
func TestKeyVaultBackend_clientCertificateCredential(t *testing.T) {
	getKeyvaultClient = func(cfg KeyVaultBackendConfig) (keyvaultClient, error) {
		s := cfg.AzureSession
		assert.Equal(t, "my-tenant-id", s.AzureTenantID)
		assert.Equal(t, "my-client-id", s.AzureClientID)
		assert.Equal(t, "/path/to/cert.pem", s.AzureClientCertificatePath)
		return &keyvaultMockClient{secrets: map[string]interface{}{}}, nil
	}
	bc := map[string]interface{}{
		"keyvaulturl": "https://my-vault.vault.azure.com/",
		"azure_session": map[string]interface{}{
			"azure_tenant_id":               "my-tenant-id",
			"azure_client_id":               "my-client-id",
			"azure_client_certificate_path": "/path/to/cert.pem",
		},
	}
	_, err := NewKeyVaultBackend(bc)
	assert.NoError(t, err)
}

// TestKeyVaultBackend_defaultCredential verifies that omitting all auth fields
// falls through to the default credential path.
func TestKeyVaultBackend_defaultCredential(t *testing.T) {
	getKeyvaultClient = func(cfg KeyVaultBackendConfig) (keyvaultClient, error) {
		s := cfg.AzureSession
		assert.Empty(t, s.AzureClientID)
		assert.Empty(t, s.AzureTenantID)
		assert.Empty(t, s.AzureClientSecret)
		return &keyvaultMockClient{secrets: map[string]interface{}{}}, nil
	}
	bc := map[string]interface{}{
		"keyvaulturl": "https://my-vault.vault.azure.com/",
	}
	_, err := NewKeyVaultBackend(bc)
	assert.NoError(t, err)
}

// TestKeyVaultBackend_topLevelClientIDAlias verifies that top-level "clientid" is
// mapped to azure_session.azure_client_id when azure_session is used.
func TestKeyVaultBackend_topLevelClientIDAlias(t *testing.T) {
	getKeyvaultClient = func(cfg KeyVaultBackendConfig) (keyvaultClient, error) {
		assert.Equal(t, "123abc45-123a-abcd-123a-123abc456def", cfg.AzureSession.AzureClientID)
		return &keyvaultMockClient{secrets: map[string]interface{}{}}, nil
	}
	bc := map[string]interface{}{
		"keyvaulturl": "https://my-vault.vault.azure.com/",
		"clientid":    "123abc45-123a-abcd-123a-123abc456def",
	}
	_, err := NewKeyVaultBackend(bc)
	assert.NoError(t, err)
}
