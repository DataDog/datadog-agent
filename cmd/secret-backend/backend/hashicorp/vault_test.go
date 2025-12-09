// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package hashicorp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/aws"
	vaultHttp "github.com/hashicorp/vault/http"
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

	ctx := context.Background()
	secretOutput := secretsBackend.GetSecretOutput(ctx, "secret/foo;key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput(ctx, "secret/foo;key2")
	assert.Equal(t, "value2", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput(ctx, "secret/foo;key_noexist")
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

	ctx := context.Background()
	secretOutput := secretsBackend.GetSecretOutput(ctx, "secret/foo;key_noexist")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func createTestVault(t *testing.T) (net.Listener, *api.Client, string) {
	t.Helper()

	// Create an in-memory, unsealed core (the "backend", if you will).
	core, keyShares, rootToken := vault.TestCoreUnsealed(t)
	_ = keyShares

	// Start an HTTP server for the core.
	ln, addr := vaultHttp.TestServer(t, core)

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

func TestNewAuthenticationFromBackendConfig_AWSAuth(t *testing.T) {
	client, err := api.NewClient(api.DefaultConfig())
	require.NoError(t, err)

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
			},
			expectAuth:  false,
			expectError: false,
		},
		{
			name: "Non-AWS auth type should not create AWS auth",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "userpass",
				VaultAWSRole:  "test-role",
			},
			expectAuth:  false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backendConfig := VaultBackendConfig{VaultSession: tt.sessionConfig}
			auth, _, err := newAuthenticationFromBackendConfig(backendConfig, client)

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

	ctx := context.Background()
	secretOutput := secretsBackend.GetSecretOutput(ctx, "kv2/foo;key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput(ctx, "kv2/foo;key2")
	assert.Equal(t, "value2", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput(ctx, "kv2/foo;not_there")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestGetKubernetesJWTToken(t *testing.T) {
	tests := []struct {
		name            string
		sessionConfig   VaultSessionBackendConfig
		envVars         map[string]string
		createTempFile  bool
		tempFileContent string
		expectError     bool
		expectedToken   string
	}{
		{
			name: "Direct JWT token",
			sessionConfig: VaultSessionBackendConfig{
				VaultKubernetesJWT: "direct-jwt-token",
			},
			expectError:   false,
			expectedToken: "direct-jwt-token",
		},
		{
			name: "JWT from file path",
			sessionConfig: VaultSessionBackendConfig{
				VaultKubernetesJWTPath: "tmp", // Will be set to temp file
			},
			createTempFile:  true,
			tempFileContent: "file-jwt-token",
			expectError:     false,
			expectedToken:   "file-jwt-token",
		},
		{
			name: "JWT from file path with whitespace",
			sessionConfig: VaultSessionBackendConfig{
				VaultKubernetesJWTPath: "tmp", // Will be set to temp file
			},
			createTempFile:  true,
			tempFileContent: "  file-jwt-token-with-spaces  \n",
			expectError:     false,
			expectedToken:   "file-jwt-token-with-spaces",
		},
		{
			name:          "JWT from default path via env var",
			sessionConfig: VaultSessionBackendConfig{
				// No explicit path set
			},
			envVars: map[string]string{
				"DD_SECRETS_SA_TOKEN_PATH": "tmp", // Will be set to temp file
			},
			createTempFile:  true,
			tempFileContent: "default-env-path-token",
			expectError:     false,
			expectedToken:   "default-env-path-token",
		},
		{
			name: "JWT from non-existent file",
			sessionConfig: VaultSessionBackendConfig{
				VaultKubernetesJWTPath: "/non/existent/file",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file if needed
			if tt.createTempFile {
				tmpFile, err := os.CreateTemp("", "jwt-token-test")
				require.NoError(t, err)
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tt.tempFileContent)
				require.NoError(t, err)
				tmpFile.Close()

				if tt.sessionConfig.VaultKubernetesJWTPath == "tmp" {
					tt.sessionConfig.VaultKubernetesJWTPath = tmpFile.Name()
				}
				if tt.envVars["DD_SECRETS_SA_TOKEN_PATH"] == "tmp" {
					t.Setenv("DD_SECRETS_SA_TOKEN_PATH", tmpFile.Name())
				}
			}

			token, err := getKubernetesJWTToken(tt.sessionConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedToken, token)
			}
		})
	}
}

func TestNewVaultBackend_KubernetesAuth(t *testing.T) {
	ln, _, _ := createTestVault(t)
	defer ln.Close()

	tmpFile, err := os.CreateTemp("", "jwt-token-test")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("test-jwt-token")
	require.NoError(t, err)
	tmpFile.Close()

	tests := []struct {
		name          string
		sessionConfig VaultSessionBackendConfig
		envVars       map[string]string
		errorContains string
	}{
		{
			name: "Kubernetes auth with role and direct JWT (expected failure)",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				VaultKubernetesJWT:  "test-jwt-token",
			},
			errorContains: "failed to authenticate to Vault",
		},
		{
			name: "Kubernetes auth with role from env var (expected failure)",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:      "kubernetes",
				VaultKubernetesJWT: "test-jwt-token",
			},
			envVars: map[string]string{
				"DD_SECRETS_VAULT_ROLE": "env-role",
			},
			errorContains: "failed to authenticate to Vault",
		},
		{
			name: "Kubernetes auth with JWT from file (expected failure)",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:          "kubernetes",
				VaultKubernetesRole:    "test-role",
				VaultKubernetesJWTPath: tmpFile.Name(),
			},
			errorContains: "failed to authenticate to Vault",
		},
		{
			name: "Kubernetes auth without role (should error immediately)",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:      "kubernetes",
				VaultKubernetesJWT: "test-jwt-token",
			},
			errorContains: "kubernetes role not specified",
		},
		{
			name: "Kubernetes auth success path (mocked)",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				VaultKubernetesJWT:  "test-jwt-token",
			},
			envVars: map[string]string{
				"DD_SECRETS_VAULT_AUTH_PATH": "auth/k8s/success/login",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an httptest.Server that mimics Vault’s API.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/login") {
					if tt.errorContains == "" {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"auth":{"client_token":"fake-token"}}`))
						return
					}
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors":["failed to authenticate to Vault"]}`))
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()

			// Point the agent at our mock Vault.
			t.Setenv("VAULT_ADDR", server.URL)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			backendConfig := map[string]interface{}{
				"vault_address": server.URL,
				"backend_type":  "hashicorp.vault",
				"vault_session": tt.sessionConfig,
			}

			vb, err := NewVaultBackend(backendConfig)

			if tt.errorContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				require.NotNil(t, vb)
				assert.Equal(t, "fake-token", vb.Client.Token())
			}

			// regression check: the "unsupported protocol scheme" bug should never appear
			if err != nil {
				assert.NotContains(t, err.Error(), `unsupported protocol scheme ""`)
			}
		})
	}
}

func TestVaultBackend_VaultURIFormat(t *testing.T) {
	ln, client, token := createTestVault(t)
	defer ln.Close()

	// Create test data - KV v1 stores data directly (no nested "data" field)
	complexData := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"nested": map[string]interface{}{
			"subkey1": "subvalue1",
			"subkey2": "subvalue2",
		},
		"array": []interface{}{"item1", "item2", "item3"},
	}

	_, err := client.Logical().Write("secret/complex", complexData)
	assert.NoError(t, err)

	// Also create a simple secret for basic testing
	_, err = client.Logical().Write("secret/simple", map[string]interface{}{
		"key1": "simple_value1",
		"key2": "simple_value2",
	})
	assert.NoError(t, err)

	backendConfig := map[string]interface{}{
		"vault_address": client.Address(),
		"backend_type":  "hashicorp.vault",
		"vault_token":   token,
	}

	secretsBackend, err := NewVaultBackend(backendConfig)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		secretString   string
		expectedValue  string
		expectError    bool
		errorSubstring string
	}{
		{
			name:          "Basic data access - direct key",
			secretString:  "vault://secret/complex#/key1",
			expectedValue: "value1",
			expectError:   false,
		},
		{
			name:          "Nested object access",
			secretString:  "vault://secret/complex#/nested/subkey1",
			expectedValue: "subvalue1",
			expectError:   false,
		},
		{
			name:          "Array access by index",
			secretString:  "vault://secret/complex#/array/0",
			expectedValue: "item1",
			expectError:   false,
		},
		{
			name:          "Array access by index - last element",
			secretString:  "vault://secret/complex#/array/2",
			expectedValue: "item3",
			expectError:   false,
		},
		{
			name:          "Access from data field (KV v1 stores in data)",
			secretString:  "vault://secret/simple#/data/key1",
			expectedValue: "simple_value1",
			expectError:   false,
		},
		{
			name:          "Access lease_duration",
			secretString:  "vault://secret/simple#/lease_duration",
			expectedValue: "2764800", // Changed from "0" - Vault's actual default lease duration
			expectError:   false,
		},
		{
			name:          "Access renewable field",
			secretString:  "vault://secret/simple#/renewable",
			expectedValue: "false", // Default renewable value
			expectError:   false,
		},
		{
			name:           "Invalid URI format - no hash",
			secretString:   "vault://secret/simple",
			expectError:    true,
			errorSubstring: "invalid vault:// format",
		},
		{
			name:          "Invalid URI format - multiple hashes",
			secretString:  "vault://secret/simple#/data#/key1",
			expectError:   true,
			expectedValue: "invalid JSON pointer",
		},
		{
			name:           "Invalid JSON pointer",
			secretString:   "vault://secret/simple#invalid-pointer",
			expectError:    true,
			errorSubstring: "invalid JSON pointer",
		},
		{
			name:           "Non-existent path",
			secretString:   "vault://secret/nonexistent#/data/key1",
			expectError:    true,
			errorSubstring: "secret data is nil",
		},
		{
			name:           "Non-existent key in valid path",
			secretString:   "vault://secret/simple#/data/nonexistent",
			expectError:    true,
			errorSubstring: "no value found for pointer",
		},
		{
			name:           "Unsupported pointer key",
			secretString:   "vault://secret/simple#/unsupported_field",
			expectError:    true,
			errorSubstring: "no value found for pointer",
		},
		{
			name:           "Array index out of bounds",
			secretString:   "vault://secret/complex#/array/10",
			expectError:    true,
			errorSubstring: "no value found for pointer",
		},
		{
			name:           "/data in the wrong place",
			secretString:   "vault://secret/complex/data#/value",
			expectError:    true,
			errorSubstring: "secret data is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			secretOutput := secretsBackend.GetSecretOutput(ctx, tt.secretString)

			if tt.expectError {
				assert.Nil(t, secretOutput.Value)
				require.ErrorContains(t, errors.New(*secretOutput.Error), tt.errorSubstring)
			} else {
				assert.NotNil(t, secretOutput.Value)
				assert.Nil(t, secretOutput.Error)
				assert.Equal(t, tt.expectedValue, *secretOutput.Value)
			}
		})
	}
}

func TestVaultBackend_VaultURIFormat_KVv2(t *testing.T) {
	ln, client, token := createTestVault(t)
	defer ln.Close()

	// Set up KV v2 mount
	err := client.Sys().Mount("kv2/", &api.MountInput{
		Type: "kv",
		Options: map[string]string{
			"version": "2",
		},
	})
	assert.NoError(t, err)

	// Create test data in KV v2 format
	_, err = client.Logical().Write("kv2/data/complex", map[string]interface{}{
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"nested": map[string]interface{}{
				"subkey1": "subvalue1",
				"subkey2": "subvalue2",
			},
			"array": []interface{}{"item1", "item2", "item3"},
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

	tests := []struct {
		name           string
		secretString   string
		expectedValue  string
		expectError    bool
		errorSubstring string
	}{
		{
			name:          "KV v2 basic data access",
			secretString:  "vault://kv2/data/complex#/data/key1",
			expectedValue: "value1",
			expectError:   false,
		},
		{
			name:          "KV v2 nested object access",
			secretString:  "vault://kv2/data/complex#/data/nested/subkey1",
			expectedValue: "subvalue1",
			expectError:   false,
		},
		{
			name:          "KV v2 array access",
			secretString:  "vault://kv2/data/complex#/data/array/0",
			expectedValue: "item1",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			secretOutput := secretsBackend.GetSecretOutput(ctx, tt.secretString)

			if tt.expectError {
				assert.Nil(t, secretOutput.Value)
				require.ErrorContains(t, errors.New(*secretOutput.Error), tt.errorSubstring)
			} else {
				assert.NotNil(t, secretOutput.Value)
				assert.Nil(t, secretOutput.Error)
				assert.Equal(t, tt.expectedValue, *secretOutput.Value)
			}
		})
	}
}

func TestVaultBackend_BackwardCompatibility(t *testing.T) {
	ln, client, token := createTestVault(t)
	defer ln.Close()

	// Create test data
	_, err := client.Logical().Write("secret/test", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})
	assert.NoError(t, err)

	backendConfig := map[string]interface{}{
		"vault_address": client.Address(),
		"backend_type":  "hashicorp.vault",
		"vault_token":   token,
	}

	secretsBackend, err := NewVaultBackend(backendConfig)
	assert.NoError(t, err)

	ctx := context.Background()
	// Test that old format still works
	secretOutput := secretsBackend.GetSecretOutput(ctx, "secret/test;key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	// Test that new format works
	secretOutput = secretsBackend.GetSecretOutput(ctx, "vault://secret/test#/key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}

func TestVaultBackend_ErrorHandling(t *testing.T) {
	ln, client, token := createTestVault(t)
	defer ln.Close()

	backendConfig := map[string]interface{}{
		"vault_address": client.Address(),
		"backend_type":  "hashicorp.vault",
		"vault_token":   token,
	}

	secretsBackend, err := NewVaultBackend(backendConfig)
	assert.NoError(t, err)

	tests := []struct {
		name         string
		secretString string
		expectError  bool
		errorMessage string
	}{
		{
			name:         "Invalid traditional format - no semicolon",
			secretString: "secret/test",
			expectError:  true,
			errorMessage: "invalid secret format, expected 'secret_path;key' or 'vault://path#/json/pointer'",
		},
		{
			name:         "Invalid traditional format - empty path",
			secretString: ";key1",
			expectError:  true,
		},
		{
			name:         "Invalid traditional format - empty key",
			secretString: "secret/test;",
			expectError:  true,
		},
		{
			name:         "Invalid vault URI - no hash",
			secretString: "vault://secret/test",
			expectError:  true,
			errorMessage: "invalid vault:// format, expected 'vault://path#/json/pointer'",
		},
		{
			name:         "Invalid vault URI - empty path",
			secretString: "vault://#/key1",
			expectError:  true,
		},
		{
			name:         "Invalid vault URI - empty pointer",
			secretString: "vault://secret/test#",
			expectError:  true,
			errorMessage: "invalid JSON pointer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			secretOutput := secretsBackend.GetSecretOutput(ctx, tt.secretString)

			if tt.expectError {
				assert.Nil(t, secretOutput.Value)
				require.ErrorContains(t, errors.New(*secretOutput.Error), tt.errorMessage)
			} else {
				assert.NotNil(t, secretOutput.Value)
				assert.Nil(t, secretOutput.Error)
			}
		})
	}
}

func TestNewAuthenticationFromBackendConfig_ImplicitAuth(t *testing.T) {
	client, err := api.NewClient(api.DefaultConfig())
	require.NoError(t, err)

	tests := map[string]struct {
		sessionConfig VaultSessionBackendConfig
		envValue      string
	}{
		"implicit auth from config set to 'true'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				ImplicitAuth:        "true",
			},
		},
		"implicit auth from config set to 'TRUE'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				ImplicitAuth:        "TRUE",
			},
		},
		"implicit auth from config set to 't'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				ImplicitAuth:        "t",
			},
		},
		"implicit auth from config set to 'T'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				ImplicitAuth:        "T",
			},
		},
		"implicit auth from config set to '1'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				ImplicitAuth:        "1",
			},
		},
		"implicit auth from env set to 'true'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
			},
			envValue: "true",
		},
		"implicit auth from env set to 'TRUE'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
			},
			envValue: "TRUE",
		},
		"implicit auth from env set to 't'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
			},
			envValue: "t",
		},
		"implicit auth from env set to 'T'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
			},
			envValue: "T",
		},
		"implicit auth from env set to '1'": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
			},
			envValue: "1",
		},
		"env var takes precedence over config": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:       "kubernetes",
				VaultKubernetesRole: "test-role",
				ImplicitAuth:        "false",
			},
			envValue: "true",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("DD_SECRETS_IMPLICIT_AUTH", tt.envValue)
			}

			backendConfig := VaultBackendConfig{VaultSession: tt.sessionConfig}
			auth, token, err := newAuthenticationFromBackendConfig(backendConfig, client)

			assert.NoError(t, err)
			assert.Nil(t, auth)
			assert.Equal(t, implicitAuthToken, token)
		})
	}
}

func TestNewAuthenticationFromBackendConfig_OtherAuthMethods(t *testing.T) {
	client, err := api.NewClient(api.DefaultConfig())
	require.NoError(t, err)

	tests := map[string]struct {
		sessionConfig VaultSessionBackendConfig
		expectedAuth  bool
		expectedToken string
		expectError   bool
		errorContains string
	}{
		"approle auth with role and secret": {
			sessionConfig: VaultSessionBackendConfig{
				VaultRoleID:   "test-role-id",
				VaultSecretID: "test-secret-id",
				ImplicitAuth:  "false",
			},
			expectedAuth:  true,
			expectedToken: "",
			expectError:   false,
		},
		"approle auth with only role ID": {
			sessionConfig: VaultSessionBackendConfig{
				VaultRoleID:  "test-role-id",
				ImplicitAuth: "false",
			},
			expectedAuth:  false,
			expectedToken: "",
			expectError:   false,
		},
		"aws auth with role": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "aws",
				VaultAWSRole:  "test-role",
				ImplicitAuth:  "false",
			},
			expectedAuth:  true,
			expectedToken: "",
			expectError:   false,
		},
		"unsupported auth type": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "unsupported",
				ImplicitAuth:  "false",
			},
			expectedAuth:  false,
			expectedToken: "",
			expectError:   false,
		},
		"no auth configuration": {
			sessionConfig: VaultSessionBackendConfig{},
			expectedAuth:  false,
			expectedToken: "",
			expectError:   false,
		},
		"implicit auth disabled with other auth": {
			sessionConfig: VaultSessionBackendConfig{
				VaultRoleID:   "test-role-id",
				VaultSecretID: "test-secret-id",
				ImplicitAuth:  "false",
			},
			expectedAuth:  true,
			expectedToken: "",
			expectError:   false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			backendConfig := VaultBackendConfig{VaultSession: tt.sessionConfig}
			auth, token, err := newAuthenticationFromBackendConfig(backendConfig, client)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedAuth {
				assert.NotNil(t, auth)
			} else {
				assert.Nil(t, auth)
			}

			assert.Equal(t, tt.expectedToken, token)
		})
	}
}
