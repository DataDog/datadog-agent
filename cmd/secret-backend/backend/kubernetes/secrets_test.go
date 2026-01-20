// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package kubernetes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMockK8sServer creates a test HTTP server that mimics K8s API
func createMockK8sServer() *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(k8sStatusResponse{
				Message: "Unauthorized",
				Reason:  "Unauthorized",
				Code:    401,
			})
			return
		}

		switch r.URL.Path {
		case "/api/v1/namespaces/secrets-x/secrets/my-secrets":
			_ = json.NewEncoder(w).Encode(k8sSecretResponse{
				Data: map[string][]byte{
					"password": []byte("password"),
					"username": []byte("admin"),
					"api_key":  []byte("key-123"),
				},
			})
		case "/api/v1/namespaces/secrets-y/secrets/db-secrets":
			_ = json.NewEncoder(w).Encode(k8sSecretResponse{
				Data: map[string][]byte{
					"password": []byte("db-password"),
					"host":     []byte("localhost"),
				},
			})
		case "/api/v1/namespaces/test-ns/secrets/empty-secret":
			_ = json.NewEncoder(w).Encode(k8sSecretResponse{
				Data: nil,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(k8sStatusResponse{
				Message: `secrets "unknown" not found`,
				Reason:  "NotFound",
				Code:    404,
			})
		}
	}))
}

func TestGetSecretOutput(t *testing.T) {
	server := createMockK8sServer()
	defer server.Close()

	backend := &SecretsBackend{
		HTTPClient: server.Client(),
		K8sConfig: k8sConfig{
			Host:        server.URL,
			BearerToken: "test-token",
		},
	}

	tests := []struct {
		name          string
		secretString  string
		expectError   bool
		expectedValue string
		errorContains string
	}{
		{
			name:          "valid secret",
			secretString:  "secrets-x/my-secrets;password",
			expectError:   false,
			expectedValue: "password",
		},
		{
			name:          "valid secret different key",
			secretString:  "secrets-x/my-secrets;username",
			expectError:   false,
			expectedValue: "admin",
		},
		{
			name:          "different namespace",
			secretString:  "secrets-y/db-secrets;password",
			expectError:   false,
			expectedValue: "db-password",
		},
		{
			name:          "different namespace different key",
			secretString:  "secrets-y/db-secrets;host",
			expectError:   false,
			expectedValue: "localhost",
		},
		{
			name:          "invalid format - missing key",
			secretString:  "secrets-x/my-secrets",
			expectError:   true,
			errorContains: "invalid secret format",
		},
		{
			name:          "invalid format - missing namespace",
			secretString:  "my-secrets;password",
			expectError:   true,
			errorContains: "invalid secret format",
		},
		{
			name:          "secret not found",
			secretString:  "secrets-x/nonexistent;password",
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "namespace not found",
			secretString:  "nonexistent-ns/my-secrets;password",
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "key not found in secret",
			secretString:  "secrets-x/my-secrets;nonexistent",
			expectError:   true,
			errorContains: "backend does not provide secret key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			output := backend.GetSecretOutput(ctx, tt.secretString)

			if tt.expectError {
				assert.NotNil(t, output.Error)
				assert.Nil(t, output.Value)
				if tt.errorContains != "" {
					assert.Contains(t, *output.Error, tt.errorContains)
				}
			} else {
				assert.Nil(t, output.Error)
				assert.NotNil(t, output.Value)
				assert.Equal(t, tt.expectedValue, *output.Value)
			}
		})
	}
}

func TestGetSecretOutputEmptyData(t *testing.T) {
	server := createMockK8sServer()
	defer server.Close()

	backend := &SecretsBackend{
		HTTPClient: server.Client(),
		K8sConfig: k8sConfig{
			Host:        server.URL,
			BearerToken: "test-token",
		},
	}

	ctx := context.Background()
	output := backend.GetSecretOutput(ctx, "test-ns/empty-secret;password")

	assert.NotNil(t, output.Error)
	assert.Nil(t, output.Value)
	assert.Contains(t, *output.Error, "has no data")
}

func TestGetSecretOutputEdgeCases(t *testing.T) {
	server := createMockK8sServer()
	defer server.Close()

	backend := &SecretsBackend{
		HTTPClient: server.Client(),
		K8sConfig: k8sConfig{
			Host:        server.URL,
			BearerToken: "test-token",
		},
	}

	tests := []struct {
		name          string
		secretString  string
		expectError   bool
		errorContains string
	}{
		{
			name:          "multiple semicolons",
			secretString:  "secrets-x/my-secrets;password;extra",
			expectError:   true,
			errorContains: "backend does not provide secret key",
		},
		{
			name:          "multiple slashes",
			secretString:  "secrets-x/sub/path;password",
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "double slash",
			secretString:  "secrets-x//my-secrets;password",
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "empty secret name after slash",
			secretString:  "secrets-x/;password",
			expectError:   true,
			errorContains: "cannot be empty",
		},
		{
			name:          "missing namespace before slash",
			secretString:  "/my-secrets;password",
			expectError:   true,
			errorContains: "cannot be empty",
		},
		{
			name:          "missing everything before semicolon",
			secretString:  ";password",
			expectError:   true,
			errorContains: "invalid secret format",
		},
		{
			name:          "empty key after semicolon",
			secretString:  "secrets-x/my-secrets;",
			expectError:   true,
			errorContains: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			output := backend.GetSecretOutput(ctx, tt.secretString)

			assert.NotNil(t, output.Error)
			assert.Nil(t, output.Value)
			if tt.errorContains != "" {
				assert.Contains(t, *output.Error, tt.errorContains)
			}
		})
	}
}

func certify() ([]byte, error) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-ca",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	cert, _ := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})

	return certPEM, nil
}

// TestNewSecretsBackendConfigOptions tests configuration options
func TestNewSecretsBackendConfigOptions(t *testing.T) {
	tmpDir := t.TempDir()

	tokenPath := filepath.Join(tmpDir, "token")
	caPath := filepath.Join(tmpDir, "ca.crt")

	testToken := "test-service-account-token"
	testCA, err := certify()
	require.NoError(t, err)

	err = os.WriteFile(tokenPath, []byte(testToken), 0600)
	require.NoError(t, err)
	err = os.WriteFile(caPath, testCA, 0600)
	require.NoError(t, err)

	os.Setenv("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc")
	os.Setenv("KUBERNETES_SERVICE_PORT", "443")
	t.Cleanup(func() {
		require.NoError(t, os.Unsetenv("KUBERNETES_SERVICE_HOST"))
		require.NoError(t, os.Unsetenv("KUBERNETES_SERVICE_PORT"))
	})

	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		validate    func(*testing.T, *SecretsBackend)
	}{
		{
			name: "default paths with env vars",
			config: map[string]interface{}{
				"token_path": tokenPath,
				"ca_path":    caPath,
			},
			expectError: false,
			validate: func(t *testing.T, backend *SecretsBackend) {
				assert.Equal(t, "https://kubernetes.default.svc:443", backend.K8sConfig.Host)
				assert.Equal(t, testToken, backend.K8sConfig.BearerToken)
				assert.Equal(t, testCA, backend.K8sConfig.CA)
			},
		},
		{
			name: "custom api_server override",
			config: map[string]interface{}{
				"token_path": tokenPath,
				"ca_path":    caPath,
				"api_server": "https://custom-api.example.com:6443",
			},
			expectError: false,
			validate: func(t *testing.T, backend *SecretsBackend) {
				assert.Equal(t, "https://custom-api.example.com:6443", backend.K8sConfig.Host)
				assert.Equal(t, testToken, backend.K8sConfig.BearerToken)
			},
		},
		{
			name: "all custom paths",
			config: map[string]interface{}{
				"token_path": tokenPath,
				"ca_path":    caPath,
				"api_server": "https://192.168.1.1:8443",
			},
			expectError: false,
			validate: func(t *testing.T, backend *SecretsBackend) {
				assert.Equal(t, "https://192.168.1.1:8443", backend.K8sConfig.Host)
				assert.NotNil(t, backend.HTTPClient)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewK8sSecretsBackend(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, backend)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, backend)
				if tt.validate != nil && backend != nil {
					tt.validate(t, backend)
				}
			}
		})
	}
}
