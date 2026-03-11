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
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

// mockVault holds the state for a mock Vault HTTP server.
type mockVault struct {
	mu     sync.RWMutex
	token  string
	kvv1   map[string]map[string]interface{} // path -> data
	kvv2   map[string]map[string]interface{} // path -> data (inner)
	mounts map[string]*vaultMountOutput
}

func newMockVault() *mockVault {
	return &mockVault{
		token: "test-root-token",
		kvv1:  make(map[string]map[string]interface{}),
		kvv2:  make(map[string]map[string]interface{}),
		mounts: map[string]*vaultMountOutput{
			"secret/": {Type: "kv", Options: map[string]string{"version": "1"}},
			"kv2/":    {Type: "kv", Options: map[string]string{"version": "2"}},
		},
	}
}

func (m *mockVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/")

	// Auth endpoints
	if strings.HasPrefix(path, "auth/") {
		m.handleAuth(w, r, path)
		return
	}

	// sys/mounts
	if path == "sys/mounts" {
		m.handleMounts(w)
		return
	}

	// KV v2 data paths
	if strings.HasPrefix(path, "kv2/data/") {
		m.handleKVv2(w, r, path)
		return
	}

	// KV v1 paths (under secret/)
	m.handleKVv1(w, r, path)
}

func (m *mockVault) handleAuth(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPut {
		http.NotFound(w, r)
		return
	}

	// Parse body for validation
	var body map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	// approle login
	if path == "auth/approle/login" {
		if body["role_id"] == nil || body["secret_id"] == nil {
			writeVaultError(w, http.StatusBadRequest, "missing role_id or secret_id")
			return
		}
		writeVaultAuth(w, "approle-client-token")
		return
	}

	// userpass login
	if strings.HasPrefix(path, "auth/userpass/login/") {
		if body["password"] == nil {
			writeVaultError(w, http.StatusBadRequest, "missing password")
			return
		}
		writeVaultAuth(w, "userpass-client-token")
		return
	}

	// ldap login
	if strings.HasPrefix(path, "auth/ldap/login/") {
		if body["password"] == nil {
			writeVaultError(w, http.StatusBadRequest, "missing password")
			return
		}
		writeVaultAuth(w, "ldap-client-token")
		return
	}

	// kubernetes login (and any custom auth paths ending in /login)
	if strings.HasSuffix(path, "/login") {
		if body["jwt"] == nil || body["role"] == nil {
			writeVaultError(w, http.StatusBadRequest, "missing jwt or role")
			return
		}
		writeVaultAuth(w, "fake-token")
		return
	}

	http.NotFound(w, r)
}

func (m *mockVault) handleMounts(w http.ResponseWriter) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m.mounts)
}

func (m *mockVault) handleKVv2(w http.ResponseWriter, r *http.Request, path string) {
	dataPath := strings.TrimPrefix(path, "kv2/data/")
	m.mu.Lock()
	defer m.mu.Unlock()

	switch r.Method {
	case http.MethodPut:
		var body struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeVaultError(w, http.StatusBadRequest, "invalid body")
			return
		}
		m.kvv2[dataPath] = body.Data
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&vaultResponse{})
	case http.MethodGet:
		data, ok := m.kvv2[dataPath]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&vaultResponse{
			Data: map[string]interface{}{
				"data": data,
			},
		})
	default:
		http.NotFound(w, r)
	}
}

func (m *mockVault) handleKVv1(w http.ResponseWriter, r *http.Request, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch r.Method {
	case http.MethodPut:
		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			writeVaultError(w, http.StatusBadRequest, "invalid body")
			return
		}
		m.kvv1[path] = data
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&vaultResponse{
			LeaseDuration: 2764800,
		})
	case http.MethodGet:
		data, ok := m.kvv1[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&vaultResponse{
			LeaseDuration: 2764800,
			Data:          data,
		})
	default:
		http.NotFound(w, r)
	}
}

func writeVaultAuth(w http.ResponseWriter, token string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(&vaultResponse{
		Auth: &vaultAuth{ClientToken: token},
	})
}

func writeVaultError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []string{msg},
	})
}

// createMockVault starts an httptest server backed by a mockVault and returns
// the server URL and root token. The server is cleaned up when the test ends.
func createMockVault(t *testing.T) (string, string) {
	t.Helper()

	// clear VAULT_ADDR to ensure NewVaultBackend uses the right vault address for ci test
	t.Setenv("VAULT_ADDR", "")

	mv := newMockVault()
	server := httptest.NewServer(mv)
	t.Cleanup(server.Close)

	// Seed test data via raw HTTP calls so the mock stores them.
	client := &vaultClient{address: server.URL, token: mv.token, httpClient: server.Client()}

	// KV v1 data used by multiple tests
	ctx := context.Background()
	_, _ = client.write(ctx, "secret/foo", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})
	_, _ = client.write(ctx, "secret/complex", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"nested": map[string]interface{}{
			"subkey1": "subvalue1",
			"subkey2": "subvalue2",
		},
		"array": []interface{}{"item1", "item2", "item3"},
	})
	_, _ = client.write(ctx, "secret/simple", map[string]interface{}{
		"key1": "simple_value1",
		"key2": "simple_value2",
	})
	_, _ = client.write(ctx, "secret/test", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})

	// KV v2 data
	_, _ = client.write(ctx, "kv2/data/foo", map[string]interface{}{
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	_, _ = client.write(ctx, "kv2/data/complex", map[string]interface{}{
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

	return server.URL, mv.token
}

func TestVaultBackend(t *testing.T) {
	serverURL, token := createMockVault(t)

	backendConfig := map[string]interface{}{
		"vault_address": serverURL,
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
	serverURL, token := createMockVault(t)

	backendConfig := map[string]interface{}{
		"vault_address": serverURL,
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

func TestNewAuthenticationFromBackendConfig_AWSAuth(t *testing.T) {
	client := &vaultClient{
		address:    "http://localhost:8200",
		httpClient: http.DefaultClient,
	}

	tests := []struct {
		name          string
		sessionConfig VaultSessionBackendConfig
		expectToken   bool
		expectError   bool
	}{
		{
			name: "AWS auth type without role should return empty token",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "aws",
			},
			expectToken: false,
			expectError: false,
		},
		{
			name: "Non-AWS auth type should not trigger AWS auth",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "userpass",
				VaultAWSRole:  "test-role",
			},
			expectToken: false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backendConfig := VaultBackendConfig{VaultSession: tt.sessionConfig}
			token, err := newAuthenticationFromBackendConfig(backendConfig, client)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.expectToken {
				assert.NotEmpty(t, token)
			} else {
				assert.Empty(t, token)
			}
		})
	}
}

func TestVaultBackend_KVV2Support(t *testing.T) {
	serverURL, token := createMockVault(t)

	backendConfig := map[string]interface{}{
		"vault_address": serverURL,
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
	createMockVault(t) // Ensures VAULT_ADDR is cleared

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
			errorContains: "vault login response did not return a token",
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
			errorContains: "vault login response did not return a token",
		},
		{
			name: "Kubernetes auth with JWT from file (expected failure)",
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType:          "kubernetes",
				VaultKubernetesRole:    "test-role",
				VaultKubernetesJWTPath: tmpFile.Name(),
			},
			errorContains: "vault login response did not return a token",
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
			// Create an httptest.Server that mimics Vault's API.
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
				assert.Equal(t, "fake-token", vb.client.token)
			}

			// regression check: the "unsupported protocol scheme" bug should never appear
			if err != nil {
				assert.NotContains(t, err.Error(), `unsupported protocol scheme ""`)
			}
		})
	}
}

func TestVaultBackend_VaultURIFormat(t *testing.T) {
	serverURL, token := createMockVault(t)

	backendConfig := map[string]interface{}{
		"vault_address": serverURL,
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
	serverURL, token := createMockVault(t)

	backendConfig := map[string]interface{}{
		"vault_address": serverURL,
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
	serverURL, token := createMockVault(t)

	backendConfig := map[string]interface{}{
		"vault_address": serverURL,
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
	serverURL, token := createMockVault(t)

	backendConfig := map[string]interface{}{
		"vault_address": serverURL,
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
	client := &vaultClient{
		address:    "http://localhost:8200",
		httpClient: http.DefaultClient,
	}

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
			token, err := newAuthenticationFromBackendConfig(backendConfig, client)

			assert.NoError(t, err)
			assert.Equal(t, implicitAuthToken, token)
		})
	}
}

func TestNewAuthenticationFromBackendConfig_OtherAuthMethods(t *testing.T) {
	// Start a mock server that handles auth endpoints
	mv := newMockVault()
	server := httptest.NewServer(mv)
	defer server.Close()

	client := &vaultClient{
		address:    server.URL,
		token:      "",
		httpClient: server.Client(),
	}

	tests := map[string]struct {
		sessionConfig VaultSessionBackendConfig
		expectToken   bool
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
			expectToken:   true,
			expectedToken: "approle-client-token",
			expectError:   false,
		},
		"approle auth with only role ID": {
			sessionConfig: VaultSessionBackendConfig{
				VaultRoleID:  "test-role-id",
				ImplicitAuth: "false",
			},
			expectToken:   false,
			expectedToken: "",
			expectError:   false,
		},
		"unsupported auth type": {
			sessionConfig: VaultSessionBackendConfig{
				VaultAuthType: "unsupported",
				ImplicitAuth:  "false",
			},
			expectToken:   false,
			expectedToken: "",
			expectError:   false,
		},
		"no auth configuration": {
			sessionConfig: VaultSessionBackendConfig{},
			expectToken:   false,
			expectedToken: "",
			expectError:   false,
		},
		"implicit auth disabled with approle": {
			sessionConfig: VaultSessionBackendConfig{
				VaultRoleID:   "test-role-id",
				VaultSecretID: "test-secret-id",
				ImplicitAuth:  "false",
			},
			expectToken:   true,
			expectedToken: "approle-client-token",
			expectError:   false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			backendConfig := VaultBackendConfig{VaultSession: tt.sessionConfig}
			token, err := newAuthenticationFromBackendConfig(backendConfig, client)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.expectToken {
				assert.NotEmpty(t, token)
				if tt.expectedToken != "" {
					assert.Equal(t, tt.expectedToken, token)
				}
			} else {
				assert.Empty(t, token)
			}
		})
	}
}
