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
	"github.com/hashicorp/vault/http"
	"github.com/hashicorp/vault/vault"
	"github.com/stretchr/testify/assert"
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
		"secret_path":   "secret/foo",
		"secrets":       []string{"key1", "key2"},
		"backend_type":  "hashicorp.vault",
		// Note: we're not testing the whole "session" part of the backend here as we're using the root token.
		"vault_token": token,
	}

	secretsBackend, err := NewVaultBackend("vault-backend", backendConfig)
	assert.NoError(t, err)

	secretOutput := secretsBackend.GetSecretOutput("key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput("key_noexist")
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
		"secret_path":   "secret/foo",
		"secrets":       []string{"key_noexist"},
		"backend_type":  "hashicorp.vault",
		// Note: we're not testing the whole "session" part of the backend here as we're using the root token.
		"vault_token": token,
	}

	secretsBackend, err := NewVaultBackend("vault-backend", backendConfig)
	assert.NoError(t, err)

	// Check that the keys are not found.
	secretOutput := secretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput("key2")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput("key_noexist")
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
