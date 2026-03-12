// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package hashicorp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/vaulthttp"
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockVaultStore backs the httptest.Server that replaces the real Vault server.
type mockVaultStore struct {
	mu      sync.Mutex
	secrets map[string]map[string]interface{} // path -> data
	mounts  map[string]mountInfo
	token   string
}

type mountInfo struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
}

func newMockVaultStore(token string) *mockVaultStore {
	return &mockVaultStore{
		secrets: make(map[string]map[string]interface{}),
		mounts: map[string]mountInfo{
			"secret/": {Type: "kv", Options: map[string]string{"version": "1"}},
		},
		token: token,
	}
}

func (s *mockVaultStore) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		path := strings.TrimPrefix(r.URL.Path, "/v1/")

		// sys/mounts
		if path == "sys/mounts" && r.Method == "GET" {
			resp := make(map[string]interface{})
			resp["request_id"] = "mock"
			for k, v := range s.mounts {
				resp[k] = v
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Auth endpoints (login)
		if strings.HasSuffix(path, "/login") || strings.Contains(path, "/login/") {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			resp := map[string]interface{}{
				"auth": map[string]interface{}{
					"client_token": s.token,
				},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		switch r.Method {
		case "GET":
			data, ok := s.secrets[path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"errors":[]}`))
				return
			}
			resp := map[string]interface{}{
				"request_id":     "mock-req",
				"lease_id":       "",
				"renewable":      false,
				"lease_duration": 2764800,
				"data":           data,
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)

		case "POST", "PUT":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			s.secrets[path] = body
			w.WriteHeader(http.StatusOK)

		default:
			http.NotFound(w, r)
		}
	})
}

func (s *mockVaultStore) addMount(path string, mi mountInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mounts[path] = mi
}

// mockStores maps test name -> *mockVaultStore for tests that need to add mounts.
var mockStores sync.Map

func getStore(t *testing.T) *mockVaultStore {
	v, ok := mockStores.Load(t.Name())
	if !ok {
		// Try parent test name (for subtests).
		parts := strings.SplitN(t.Name(), "/", 2)
		if len(parts) == 2 {
			v, ok = mockStores.Load(parts[0])
		}
	}
	require.True(t, ok, "mock store not found for test %s", t.Name())
	return v.(*mockVaultStore)
}

func createTestVault(t *testing.T) (*vaulthttp.Client, string) {
	t.Helper()

	t.Setenv("VAULT_ADDR", "")

	token := "test-root-token"
	store := newMockVaultStore(token)

	server := httptest.NewServer(store.handler())
	t.Cleanup(func() { server.Close() })

	client, err := vaulthttp.NewClient(server.URL, nil)
	require.NoError(t, err)
	client.SetToken(token)

	mockStores.Store(t.Name(), store)
	t.Cleanup(func() { mockStores.Delete(t.Name()) })

	return client, token
}

func TestVaultBackend(t *testing.T) {
	client, token := createTestVault(t)

	_, err := client.Write(context.Background(), "secret/foo", map[string]interface{}{
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
	client, token := createTestVault(t)

	_, err := client.Write(context.Background(), "secret/foo", map[string]interface{}{
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
	secretOutput := secretsBackend.GetSecretOutput(ctx, "secret/foo;key_noexist")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestVaultBackend_KVV2Support(t *testing.T) {
	client, token := createTestVault(t)
	store := getStore(t)

	store.addMount("kv2/", mountInfo{Type: "kv", Options: map[string]string{"version": "2"}})

	_, err := client.Write(context.Background(), "kv2/data/foo", map[string]interface{}{
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
				VaultKubernetesJWTPath: "tmp",
			},
			createTempFile:  true,
			tempFileContent: "file-jwt-token",
			expectError:     false,
			expectedToken:   "file-jwt-token",
		},
		{
			name: "JWT from file path with whitespace",
			sessionConfig: VaultSessionBackendConfig{
				VaultKubernetesJWTPath: "tmp",
			},
			createTempFile:  true,
			tempFileContent: "  file-jwt-token-with-spaces  \n",
			expectError:     false,
			expectedToken:   "file-jwt-token-with-spaces",
		},
		{
			name:          "JWT from default path via env var",
			sessionConfig: VaultSessionBackendConfig{},
			envVars: map[string]string{
				"DD_SECRETS_SA_TOKEN_PATH": "tmp",
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
	createTestVault(t) // ensure VAULT_ADDR is cleared

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
			errorContains: "unable to extract token from Vault login response",
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
			errorContains: "unable to extract token from Vault login response",
		},
		{
			name: "Kubernetes auth with JWT from file (expected failure)",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:          "kubernetes",
				VaultKubernetesRole:    "test-role",
				VaultKubernetesJWTPath: tmpFile.Name(),
			},
			errorContains: "unable to extract token from Vault login response",
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
	client, token := createTestVault(t)

	complexData := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"nested": map[string]interface{}{
			"subkey1": "subvalue1",
			"subkey2": "subvalue2",
		},
		"array": []interface{}{"item1", "item2", "item3"},
	}

	_, err := client.Write(context.Background(), "secret/complex", complexData)
	assert.NoError(t, err)

	_, err = client.Write(context.Background(), "secret/simple", map[string]interface{}{
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
			expectedValue: "2764800",
			expectError:   false,
		},
		{
			name:          "Access renewable field",
			secretString:  "vault://secret/simple#/renewable",
			expectedValue: "false",
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
	client, token := createTestVault(t)
	store := getStore(t)

	store.addMount("kv2/", mountInfo{Type: "kv", Options: map[string]string{"version": "2"}})

	_, err := client.Write(context.Background(), "kv2/data/complex", map[string]interface{}{
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
	client, token := createTestVault(t)

	_, err := client.Write(context.Background(), "secret/test", map[string]interface{}{
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
	secretOutput := secretsBackend.GetSecretOutput(ctx, "secret/test;key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsBackend.GetSecretOutput(ctx, "vault://secret/test#/key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}

func TestVaultBackend_ErrorHandling(t *testing.T) {
	client, token := createTestVault(t)

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

func TestAuthenticateVaultClient_ImplicitAuth(t *testing.T) {
	client, err := vaulthttp.NewClient("http://127.0.0.1:8200", nil)
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
			ctx := context.Background()
			token, err := authenticateVaultClient(ctx, backendConfig, client)

			assert.NoError(t, err)
			assert.Equal(t, implicitAuthToken, token)
		})
	}
}

func TestAuthenticateVaultClient_OtherAuthMethods(t *testing.T) {
	// Create a mock server that handles auth endpoints.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/login") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"auth":{"client_token":"test-auth-token"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := vaulthttp.NewClient(server.URL, nil)
	require.NoError(t, err)

	tests := map[string]struct {
		sessionConfig VaultSessionBackendConfig
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
			expectedToken: "test-auth-token",
			expectError:   false,
		},
		"approle auth with only role ID": {
			sessionConfig: VaultSessionBackendConfig{
				VaultRoleID:  "test-role-id",
				ImplicitAuth: "false",
			},
			expectedToken: "",
			expectError:   false,
		},
		"unsupported auth type": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "unsupported",
				ImplicitAuth:  "false",
			},
			expectedToken: "",
			expectError:   false,
		},
		"no auth configuration": {
			sessionConfig: VaultSessionBackendConfig{},
			expectedToken: "",
			expectError:   false,
		},
		"implicit auth disabled with approle auth": {
			sessionConfig: VaultSessionBackendConfig{
				VaultRoleID:   "test-role-id",
				VaultSecretID: "test-secret-id",
				ImplicitAuth:  "false",
			},
			expectedToken: "test-auth-token",
			expectError:   false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			backendConfig := VaultBackendConfig{VaultSession: tt.sessionConfig}
			ctx := context.Background()
			token, err := authenticateVaultClient(ctx, backendConfig, client)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedToken, token)
		})
	}
}
