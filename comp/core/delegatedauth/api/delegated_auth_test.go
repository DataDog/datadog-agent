// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name         string
		responseJSON string
		wantAPIKey   string
		wantErr      bool
	}{
		{
			name: "valid response",
			responseJSON: `{
				"data": {
					"attributes": {
						"api_key": "test-api-key-12345"
					}
				}
			}`,
			wantAPIKey: "test-api-key-12345",
			wantErr:    false,
		},
		{
			name: "valid response with extra fields",
			responseJSON: `{
				"data": {
					"id": "some-id",
					"type": "intake-key",
					"attributes": {
						"api_key": "another-test-key",
						"created_at": "2025-01-01T00:00:00Z",
						"name": "test-key"
					}
				}
			}`,
			wantAPIKey: "another-test-key",
			wantErr:    false,
		},
		{
			name: "empty api_key",
			responseJSON: `{
				"data": {
					"attributes": {
						"api_key": ""
					}
				}
			}`,
			wantAPIKey: "",
			wantErr:    true,
		},
		{
			name: "missing attributes",
			responseJSON: `{
				"data": {
					"id": "some-id"
				}
			}`,
			wantAPIKey: "",
			wantErr:    true,
		},
		{
			name: "missing data",
			responseJSON: `{
				"error": "something went wrong"
			}`,
			wantAPIKey: "",
			wantErr:    true,
		},
		{
			name:         "invalid json",
			responseJSON: `{invalid json`,
			wantAPIKey:   "",
			wantErr:      true,
		},
		{
			name:         "empty response",
			responseJSON: `{}`,
			wantAPIKey:   "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey, err := parseResponse([]byte(tt.responseJSON))

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, apiKey)
			} else {
				require.NoError(t, err)
				require.NotNil(t, apiKey)
				assert.Equal(t, tt.wantAPIKey, *apiKey)
			}
		})
	}
}

func TestTokenResponseStructure(t *testing.T) {
	// Test that the struct fields are properly tagged and can be marshaled/unmarshaled
	response := TokenResponse{
		Data: TokenData{
			Attributes: TokenAttributes{
				APIKey: "test-key",
			},
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	// Verify JSON structure
	expectedJSON := `{"data":{"attributes":{"api_key":"test-key"}}}`
	assert.JSONEq(t, expectedJSON, string(jsonBytes))

	// Unmarshal back
	var unmarshaled TokenResponse
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, "test-key", unmarshaled.Data.Attributes.APIKey)
}

func TestGetAPIDomain(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		// Production domains
		{
			name:     "production intake domain",
			endpoint: "https://agent.datadoghq.com",
			want:     "https://api.datadoghq.com",
		},
		{
			name:     "production intake domain with trailing dot",
			endpoint: "https://agent.datadoghq.com.",
			want:     "https://api.datadoghq.com.",
		},
		{
			name:     "production EU domain",
			endpoint: "https://agent.datadoghq.eu",
			want:     "https://api.datadoghq.eu",
		},
		{
			name:     "production regional US1 domain",
			endpoint: "https://agent.us1.datadoghq.com",
			want:     "https://api.us1.datadoghq.com",
		},
		{
			name:     "production regional EU1 domain",
			endpoint: "https://metrics.eu1.datadoghq.com",
			want:     "https://api.eu1.datadoghq.com",
		},
		// Staging/internal domains (datad0g.com)
		{
			name:     "staging intake domain",
			endpoint: "https://agent.datad0g.com",
			want:     "https://api.datad0g.com",
		},
		{
			name:     "staging intake domain with trailing dot",
			endpoint: "https://agent.datad0g.com.",
			want:     "https://api.datad0g.com.",
		},
		{
			name:     "staging EU domain",
			endpoint: "https://agent.datad0g.eu",
			want:     "https://api.datad0g.eu",
		},
		{
			name:     "staging regional US1 domain",
			endpoint: "https://agent.us1.datad0g.com",
			want:     "https://api.us1.datad0g.com",
		},
		// Gov cloud
		{
			name:     "gov cloud domain",
			endpoint: "https://agent.ddog-gov.com",
			want:     "https://api.ddog-gov.com",
		},
		{
			name:     "gov cloud domain with trailing dot",
			endpoint: "https://agent.ddog-gov.com.",
			want:     "https://api.ddog-gov.com.",
		},
		// Unknown/custom domains (should pass through unchanged)
		{
			name:     "custom domain unchanged",
			endpoint: "https://custom.example.com",
			want:     "https://custom.example.com",
		},
		{
			name:     "localhost unchanged",
			endpoint: "http://localhost:8080",
			want:     "http://localhost:8080",
		},
		{
			name:     "IP address unchanged",
			endpoint: "https://192.168.1.1",
			want:     "https://192.168.1.1",
		},
		// Edge cases
		{
			name:     "already app subdomain",
			endpoint: "https://api.datadoghq.com",
			want:     "https://api.datadoghq.com",
		},
		{
			name:     "with trailing slash",
			endpoint: "https://agent.datadoghq.com/",
			want:     "https://api.datadoghq.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAPIDomain(tt.endpoint)
			assert.Equal(t, tt.want, got)
		})
	}
}
