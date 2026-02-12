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

	"github.com/DataDog/datadog-agent/pkg/config/mock"
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
		name       string
		configYAML string
		want       string
	}{
		// When dd_url is explicitly set, it takes precedence
		{
			name: "dd_url takes precedence over site",
			configYAML: `
dd_url: https://custom-api.example.com
site: datadoghq.com
`,
			want: "https://custom-api.example.com",
		},
		{
			name: "custom dd_url unchanged",
			configYAML: `
dd_url: https://custom.example.com
`,
			want: "https://custom.example.com",
		},
		{
			name: "localhost dd_url unchanged",
			configYAML: `
dd_url: http://localhost:8080
`,
			want: "http://localhost:8080",
		},
		// When site is set (dd_url empty), URL is built from prefix + site
		// Note: Well-known Datadog sites get a trailing dot (FQDN) appended
		{
			name: "production site",
			configYAML: `
site: datadoghq.com
`,
			want: "https://api.datadoghq.com.",
		},
		{
			name: "EU site",
			configYAML: `
site: datadoghq.eu
`,
			want: "https://api.datadoghq.eu.",
		},
		{
			name: "regional US1 site",
			configYAML: `
site: us1.datadoghq.com
`,
			want: "https://api.us1.datadoghq.com.",
		},
		{
			name: "regional EU1 site",
			configYAML: `
site: eu1.datadoghq.com
`,
			want: "https://api.eu1.datadoghq.com.",
		},
		{
			name: "staging site",
			configYAML: `
site: datad0g.com
`,
			want: "https://api.datad0g.com.",
		},
		{
			name: "gov cloud site",
			configYAML: `
site: ddog-gov.com
`,
			want: "https://api.ddog-gov.com.",
		},
		// Default when neither dd_url nor site is set
		{
			name:       "default site when nothing set",
			configYAML: ``,
			want:       "https://api.datadoghq.com.", // Default site is datadoghq.com with FQDN trailing dot
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg = mock.New(t)
			if tt.configYAML != "" {
				cfg = mock.NewFromYAML(t, tt.configYAML)
			}

			got := getAPIDomain(cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}
