// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package hashicorp

import (
	"net"
	"testing"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/aws"
	"github.com/hashicorp/vault/http"
	"github.com/hashicorp/vault/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultBackend(t *testing.T) {
	ln, client, token := createTestVault(t)
	defer ln.Close()

	_, err := client.Logical().Write("secret/foo", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})
	assert.NoError(t, err)

	// Create a new Vault backend.
	backendConfig := map[string]interface{}{
		"vault_address": client.Address(),
		"backend_type":  "hashicorp.vault",
		// Note: we're not testing the whole "session" part of the backend here as we're using the root token.
		"vault_token": token,
	}

	secretsBackend, err := NewVaultBackend(backendConfig)
	assert.NoError(t, err)

	secretOutput := secretsBackend.GetSecretOutput("secret/foo;key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput("secret/foo;key2")
	assert.Equal(t, "value2", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput("secret/foo;key_noexist")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestVaultBackend_KeyNotFound(t *testing.T) {
	ln, client, token := createTestVault(t)
	defer ln.Close()

	_, err := client.Logical().Write("secret/foo", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})
	assert.NoError(t, err)

	// Create a new Vault backend.
	backendConfig := map[string]interface{}{
		"vault_address": client.Address(),
		"backend_type":  "hashicorp.vault",
		// Note: we're not testing the whole "session" part of the backend here as we're using the root token.
		"vault_token": token,
	}

	secretsBackend, err := NewVaultBackend(backendConfig)
	assert.NoError(t, err)

	secretOutput := secretsBackend.GetSecretOutput("secret/foo;key_noexist")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func createTestVault(t *testing.T) (net.Listener, *api.Client, string) {
	t.Helper()

	// Create an in-memory, unsealed core (the "backend", if you will).
	core, keyShares, rootToken := vault.TestCoreUnsealed(t)
	_ = keyShares

	// Start an HTTP server for the core.
	ln, addr := http.TestServer(t, core)

	// Create a client that talks to the server, initially authenticating with
	// the root token.
	conf := api.DefaultConfig()
	conf.Address = addr

	client, err := api.NewClient(conf)
	if err != nil {
		t.Fatal(err)
	}
	client.SetToken(rootToken)

	return ln, client, rootToken
}

func TestNewVaultConfigFromBackendConfig_AWSAuth(t *testing.T) {
	tests := []struct {
		name          string
		sessionConfig VaultSessionBackendConfig
		expectAuth    bool
		expectError   bool
		validateAuth  func(t *testing.T, auth interface{})
	}{
		{
			name: "AWS auth with role only",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "aws",
				VaultAWSRole:  "test-role",
			},
			expectAuth:  true,
			expectError: false,
			validateAuth: func(t *testing.T, auth interface{}) {
				awsAuth, ok := auth.(*aws.AWSAuth)
				require.True(t, ok, "Expected AWSAuth type")
				assert.NotNil(t, awsAuth)
			},
		},
		{
			name: "AWS auth with role and region",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "aws",
				VaultAWSRole:  "test-role",
				AWSRegion:     "us-west-2",
			},
			expectAuth:  true,
			expectError: false,
			validateAuth: func(t *testing.T, auth interface{}) {
				awsAuth, ok := auth.(*aws.AWSAuth)
				require.True(t, ok, "Expected AWSAuth type")
				assert.NotNil(t, awsAuth)
			},
		},
		{
			name: "AWS auth type without role should return nil",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "aws",
				// VaultAWSRole is empty
			},
			expectAuth:  false,
			expectError: false,
		},
		{
			name: "Non-AWS auth type should not create AWS auth",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "userpass",
				VaultAWSRole:  "test-role", // This should be ignored
			},
			expectAuth:  false,
			expectError: false,
		},
		{
			name: "Empty auth type with AWS role should not create AWS auth",
			sessionConfig: VaultSessionBackendConfig{
				VaultAWSRole: "test-role",
				// VaultAuthType is empty
			},
			expectAuth:  false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := newVaultConfigFromBackendConfig(tt.sessionConfig)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.expectAuth {
				assert.NotNil(t, auth, "Expected non-nil auth method")
				if tt.validateAuth != nil {
					tt.validateAuth(t, auth)
				}
			} else {
				assert.Nil(t, auth, "Expected nil auth method")
			}
		})
	}
}

func TestVaultBackend_KVV2Support(t *testing.T) {
	ln, client, token := createTestVault(t)
	defer ln.Close()

	err := client.Sys().Mount("kv2/", &api.MountInput{
		Type: "kv",
		Options: map[string]string{
			"version": "2",
		},
	})
	assert.NoError(t, err)

	// Simulate KV v2 structure: {"data": {"key1": "value1", "key2": "value2"}, "metadata": {"something": "else"}}
	_, err = client.Logical().Write("kv2/data/foo", map[string]interface{}{
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
		"metadata": map[string]interface{}{
			"custom_metadata": "custom_value",
		},
	})
	assert.NoError(t, err)

	backendConfig := map[string]interface{}{
		"vault_address": client.Address(),
		"backend_type":  "hashicorp.vault",
		"vault_token":   token,
	}

	secretsBackend, err := NewVaultBackend(backendConfig)
	assert.NoError(t, err)

	secretOutput := secretsBackend.GetSecretOutput("kv2/foo;key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput("kv2/foo;key2")
	assert.Equal(t, "value2", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput("kv2/foo;not_there")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}
