// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockSecretManagerServer creates a test HTTP server that mocks GCP Secret Manager
func mockSecretManagerServer(secrets map[string]string) *httptest.Server {
	re := regexp.MustCompile(`secrets/([^/]+)/versions/([^:]+)`)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matches := re.FindStringSubmatch(r.URL.Path)
		if len(matches) != 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		name, version := matches[1], matches[2]
		value, ok := secrets[name+";"+version+";"]
		if !ok {
			if value, ok = secrets[name]; !ok {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
		}

		response := accessSecretVersionResponse{
			Payload: struct {
				Data       string `json:"data"`
				DataCRC32C string `json:"dataCrc32c"`
			}{
				Data: base64.StdEncoding.EncodeToString([]byte(value)),
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
}

func TestNewSecretManagerBackendInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
	}{
		{
			name:   "empty config",
			config: map[string]interface{}{},
		},
		{
			name: "missing project_id",
			config: map[string]interface{}{
				"gcp_session": map[string]interface{}{},
			},
		},
		{
			name: "empty project_id",
			config: map[string]interface{}{
				"gcp_session": map[string]interface{}{
					"project_id": "",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend, err := NewSecretManagerBackend(test.config)
			assert.Error(t, err)
			assert.Nil(t, backend)
		})
	}
}

func TestSecretManagerBackend(t *testing.T) {
	mockServer := mockSecretManagerServer(map[string]string{
		"secretX": "valueX",
		"secretY": "valueY",
	})
	defer mockServer.Close()

	backend := &SecretManagerBackend{
		Config: SecretManagerBackendConfig{
			Session: struct {
				ProjectID string `mapstructure:"project_id"`
			}{ProjectID: "test-project"},
		},
		Client: mockServer.Client(),
	}

	// overrides serviceEndpoint to point to the mock server
	defer func(url string) { serviceEndpoint = url }(serviceEndpoint)
	serviceEndpoint = mockServer.URL

	tests := []struct {
		name   string
		secret string
		value  string
		fail   bool
	}{
		{
			name:   "basic secret fetch",
			secret: "secretX",
			value:  "valueX",
			fail:   false,
		},
		{
			name:   "secret with explicit version",
			secret: "secretY;;latest",
			value:  "valueY",
			fail:   false,
		},
		{
			name:   "secret not found",
			secret: "nonexistent",
			fail:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := backend.GetSecretOutput(context.Background(), test.secret)
			if test.fail {
				assert.Nil(t, output.Value)
				assert.NotNil(t, output.Error)
			} else {
				assert.NotNil(t, output.Value)
				assert.Equal(t, test.value, *output.Value)
				assert.Nil(t, output.Error)
			}
		})
	}
}

func TestSecretManagerBackendVersioning(t *testing.T) {
	mockServer := mockSecretManagerServer(map[string]string{
		"secret;latest;": "value-latest",
		"secret;1;":      "value-v1",
		"secret;2;":      "value-v2",
	})
	defer mockServer.Close()

	backend := &SecretManagerBackend{
		Config: SecretManagerBackendConfig{
			Session: struct {
				ProjectID string `mapstructure:"project_id"`
			}{ProjectID: "test-project"},
		},
		Client: mockServer.Client(),
	}

	// overrides serviceEndpoint to point to the mock server
	defer func(url string) { serviceEndpoint = url }(serviceEndpoint)
	serviceEndpoint = mockServer.URL

	tests := []struct {
		name   string
		secret string
		value  string
	}{
		{
			name:   "default version uses latest",
			secret: "secret",
			value:  "value-latest",
		},
		{
			name:   "explicit latest version",
			secret: "secret;;latest",
			value:  "value-latest",
		},
		{
			name:   "version 1",
			secret: "secret;;1",
			value:  "value-v1",
		},
		{
			name:   "version 2",
			secret: "secret;;2",
			value:  "value-v2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := backend.GetSecretOutput(context.Background(), test.secret)
			assert.NotNil(t, output.Value)
			assert.Equal(t, test.value, *output.Value)
		})
	}
}

func TestSecretManagerBackendServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error": "permission denied"}`, http.StatusForbidden)
	}))
	defer mockServer.Close()

	backend := &SecretManagerBackend{
		Config: SecretManagerBackendConfig{
			Session: struct {
				ProjectID string `mapstructure:"project_id"`
			}{ProjectID: "test-project"},
		},
		Client: mockServer.Client(),
	}

	// overrides serviceEndpoint to point to the mock server
	defer func(url string) { serviceEndpoint = url }(serviceEndpoint)
	serviceEndpoint = mockServer.URL

	output := backend.GetSecretOutput(context.Background(), "any-secret")
	assert.Nil(t, output.Value)
	assert.NotNil(t, output.Error)
	assert.Contains(t, *output.Error, "403")
}

func TestSecretManagerBackendJSONSupport(t *testing.T) {
	valueJ := `{"key-1":"val-1","key-2":"val-2"}`

	mockServer := mockSecretManagerServer(map[string]string{
		"secretJ":         valueJ,
		"secretJ;latest;": valueJ,
		"secretJ;1;":      `{"key-1":"val-1-v1"}`,
		"secretP":         "valueP",
	})
	defer mockServer.Close()

	backend := &SecretManagerBackend{
		Config: SecretManagerBackendConfig{
			Session: struct {
				ProjectID string `mapstructure:"project_id"`
			}{ProjectID: "test-project"},
		},
		Client: mockServer.Client(),
	}

	// overrides serviceEndpoint to point to the mock server
	defer func(url string) { serviceEndpoint = url }(serviceEndpoint)
	serviceEndpoint = mockServer.URL

	tests := []struct {
		name   string
		secret string
		value  string
		fail   bool
	}{
		{
			name:   "fetch JSON secret without key returns whole JSON",
			secret: "secretJ",
			value:  valueJ,
			fail:   false,
		},
		{
			name:   "fetch JSON secret with key extracts value",
			secret: "secretJ;key-1",
			value:  "val-1",
			fail:   false,
		},
		{
			name:   "fetch JSON secret with different key",
			secret: "secretJ;key-2",
			value:  "val-2",
			fail:   false,
		},
		{
			name:   "fetch JSON secret with non-existent key fails",
			secret: "secretJ;nonexistent",
			fail:   true,
		},
		{
			name:   "fetch plain secret without key",
			secret: "secretP",
			value:  "valueP",
			fail:   false,
		},
		{
			name:   "fetch plain secret with key fails (not JSON)",
			secret: "secretP;any-key",
			fail:   true,
		},
		{
			name:   "JSON with version but no key returns whole JSON",
			secret: "secretJ;;1",
			value:  `{"key-1":"val-1-v1"}`,
			fail:   false,
		},
		{
			name:   "JSON with version and key",
			secret: "secretJ;key-1;latest",
			value:  "val-1",
			fail:   false,
		},
		{
			name:   "JSON with specific version and key",
			secret: "secretJ;key-1;1",
			value:  "val-1-v1",
			fail:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := backend.GetSecretOutput(context.Background(), test.secret)
			if test.fail {
				assert.Nil(t, output.Value)
				assert.NotNil(t, output.Error)
			} else {
				assert.NotNil(t, output.Value)
				assert.Equal(t, test.value, *output.Value)
				assert.Nil(t, output.Error)
			}
		})
	}
}
