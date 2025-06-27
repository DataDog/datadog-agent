// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestBuildRoute(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		domain    domain
		path      string
		separator separator
		versioned bool
		expected  string
	}{
		{
			name:   "basic route with dot separator",
			prefix: "install",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:      "api/v1/validate",
			separator: dot,
			expected:  "https://install.datadoghq.com/api/v1/validate",
		},
		{
			name:   "route with dash separator",
			prefix: "browser-intake",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:      "api/v2/logs",
			separator: dash,
			expected:  "https://browser-intake-datadoghq.com/api/v2/logs",
		},
		{
			name:   "prefix already has separator",
			prefix: "llmobs-intake.",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:      "api/v2/llmobs",
			separator: dot,
			expected:  "https://llmobs-intake.datadoghq.com/api/v2/llmobs",
		},
		{
			name:   "path without leading slash",
			prefix: "install",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:      "api/v1/validate",
			separator: dot,
			expected:  "https://install.datadoghq.com/api/v1/validate",
		},
		{
			name:   "path with leading slash",
			prefix: "install",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:      "/api/v1/validate",
			separator: dot,
			expected:  "https://install.datadoghq.com/api/v1/validate",
		},
		{
			name:   "custom domain",
			prefix: "install",
			domain: domain{
				site:          "datadoghq.eu",
				infraEndpoint: "https://app.datadoghq.eu",
			},
			path:      "api/v1/validate",
			separator: dot,
			expected:  "https://install.datadoghq.eu/api/v1/validate",
		},
		{
			name:   "versioned route",
			prefix: "app",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:      "api/v1/validate",
			versioned: true,
			separator: dot,
			expected:  "https://6-0-0-app.agent.datadoghq.com/api/v1/validate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpointDescription := endpointDescription{
				routePrefix: tt.prefix,
				routePath:   tt.path,
				separator:   tt.separator,
				versioned:   tt.versioned,
			}
			version.AgentVersion = "6.0.0"
			_, url := endpointDescription.buildRoute(tt.domain)
			assert.Equal(t, tt.expected, url)
		})
	}
}

func TestBuildEndpoints(t *testing.T) {
	tests := []struct {
		name                string
		endpointDescription endpointDescription
		config              map[string]string
		domains             map[string]domain
		expectedEndpoints   []resolvedEndpoint
	}{
		{
			name: "endpoint with route",
			endpointDescription: endpointDescription{
				route:  "https://custom.endpoint.com",
				method: head,
			},
			domains: map[string]domain{
				"main": {
					site:       "datadoghq.com",
					mainAPIKey: "api-key-1",
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-1",
					url:    "https://custom.endpoint.com",
				},
			},
		},
		{
			name: "endpoint with prefix and multiple domains",
			endpointDescription: endpointDescription{
				routePrefix: "install",
				method:      head,
			},
			domains: map[string]domain{
				"main": {
					site:       "datadoghq.com",
					mainAPIKey: "api-key-1",
				},
				"mrf": {
					site:       "datadoghq.eu",
					mainAPIKey: "api-key-2",
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-1",
					url:    "https://install.datadoghq.com/",
				},
				{
					apiKey: "api-key-2",
					url:    "https://install.datadoghq.eu/",
				},
			},
		},
		{
			name: "endpoint with prefix, path and separator",
			endpointDescription: endpointDescription{
				routePrefix: "browser-intake",
				routePath:   "api/v2/logs",
				method:      get,
				separator:   dash,
			},
			domains: map[string]domain{
				"main": {
					site:       "datadoghq.com",
					mainAPIKey: "api-key-1",
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-1",
					url:    "https://browser-intake-datadoghq.com/api/v2/logs",
				},
			},
		},
		{
			name: "endpoint with config prefix",
			endpointDescription: endpointDescription{
				routePrefix:  "ndm.metadata.",
				method:       get,
				configPrefix: "service.metadata.",
			},
			config: map[string]string{
				"service.metadata.api_key": "api-key-custom",
			},
			domains: map[string]domain{
				"main": {
					site:            "datadoghq.com",
					mainAPIKey:      "api-key-main",
					useCustomAPIKey: true,
				},
				"MRF": {
					site:            "datadoghq.mrf.com",
					mainAPIKey:      "api-key-mrf",
					useCustomAPIKey: false,
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-custom",
					url:    "https://ndm.metadata.datadoghq.com/",
				},
				{
					apiKey: "api-key-mrf",
					url:    "https://ndm.metadata.datadoghq.mrf.com/",
				},
			},
		},
		{
			name: "endpoint with config prefix and no api key",
			endpointDescription: endpointDescription{
				routePrefix:  "ndm.metadata.",
				method:       get,
				configPrefix: "service.metadata.",
			},
			domains: map[string]domain{
				"main": {
					site:       "datadoghq.com",
					mainAPIKey: "api-key-1",
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-1",
					url:    "https://ndm.metadata.datadoghq.com/",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)

			for key, value := range tt.config {
				mockConfig.SetWithoutSource(key, value)
			}
			endpoints := tt.endpointDescription.buildEndpoints(mockConfig, tt.domains)

			assert.Len(t, endpoints, len(tt.expectedEndpoints))

			for _, expectedEndpoint := range tt.expectedEndpoints {
				found := false
				for _, endpoint := range endpoints {
					if expectedEndpoint.url == endpoint.url {
						found = true
						assert.Equal(t, tt.endpointDescription.method, endpoint.method)
						assert.Equal(t, expectedEndpoint.apiKey, endpoint.apiKey)
						break
					}
				}
				assert.True(t, found, "Expected URL %s not found in endpoints", expectedEndpoint.url)
			}
		})
	}
}

func TestGetDomainInfo(t *testing.T) {
	tests := []struct {
		name                      string
		apiKey                    string
		site                      string
		expected                  string
		multiRegionFailover       bool
		multiRegionFailoverSite   string
		multiRegionFailoverAPIKey string
		expectedKeys              map[string]domain
	}{
		{
			name:     "main site",
			apiKey:   "test-api-key",
			site:     "datadOg.com",
			expected: "datadOg.com",
			expectedKeys: map[string]domain{
				"main": {
					site:            "datadOg.com",
					mainAPIKey:      "test-api-key",
					infraEndpoint:   "https://app.datadOg.com",
					useCustomAPIKey: true,
				},
			},
		},
		{
			name:     "default site",
			apiKey:   "test-api-key",
			expected: "datadoghq.com",
			expectedKeys: map[string]domain{
				"main": {
					site:            "datadoghq.com",
					mainAPIKey:      "test-api-key",
					infraEndpoint:   "https://app.datadoghq.com.",
					useCustomAPIKey: true,
				},
			},
		},
		{
			name:                      "main and MRF",
			apiKey:                    "test-api-key",
			site:                      "datadoghq.eu",
			multiRegionFailover:       true,
			multiRegionFailoverSite:   "datadoghq.com",
			multiRegionFailoverAPIKey: "test-api-key",
			expected:                  "datadoghq.com",
			expectedKeys: map[string]domain{
				"main": {
					site:            "datadoghq.eu",
					mainAPIKey:      "test-api-key",
					infraEndpoint:   "https://app.datadoghq.eu.",
					useCustomAPIKey: true,
				},
				"MRF": {
					site:            "datadoghq.com",
					mainAPIKey:      "test-api-key",
					infraEndpoint:   "https://app.datadoghq.com.",
					useCustomAPIKey: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("api_key", tt.apiKey)
			mockConfig.SetWithoutSource("site", tt.site)
			mockConfig.SetWithoutSource("multi_region_failover.enabled", tt.multiRegionFailover)
			mockConfig.SetWithoutSource("multi_region_failover.site", tt.multiRegionFailoverSite)
			mockConfig.SetWithoutSource("multi_region_failover.api_key", tt.multiRegionFailoverAPIKey)
			domains := getDomains(mockConfig)
			assert.Equal(t, tt.expectedKeys, domains)
		})
	}
}

func TestCheckGet(t *testing.T) {
	// Create a test server that returns different status codes
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if API key is present
		apiKey := r.Header.Get("DD-API-KEY")
		if apiKey == "test-api-key" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
		}
	}))
	defer ts.Close()

	// Create a resolvedEndpoint with GET method
	endpoint := resolvedEndpoint{
		url:    ts.URL,
		base:   ts.URL,
		method: get,
		apiKey: "test-api-key",
	}

	// Create HTTP client
	client := &http.Client{}

	// Test successful GET request
	result, err := endpoint.checkGet(client)
	assert.NoError(t, err)
	assert.Equal(t, "Success", result)

	// Test with wrong API key
	endpoint.apiKey = "wrong-api-key"
	_, err = endpoint.checkGet(client)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status code: 401")
}

func TestCheckGetConnectionFailure(t *testing.T) {
	// Create a resolvedEndpoint with an invalid URL
	endpoint := resolvedEndpoint{
		url:    "http://invalid-url-that-does-not-exist.com",
		base:   "http://invalid-url-that-does-not-exist.com",
		method: get,
		apiKey: "test-api-key",
	}

	// Create HTTP client with short timeout
	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	// Test connection failure
	result, err := endpoint.checkGet(client)
	assert.Error(t, err)
	assert.Contains(t, result, "Failed to connect")
	assert.Contains(t, err.Error(), "Unable to resolve the address")
	assert.Contains(t, err.Error(), "no such host")
}

func TestCheckHeadWithRedirectLimit(t *testing.T) {
	// Create a test server that always returns a 307 Temporary Redirect
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusTemporaryRedirect)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("OK"))
		}
	}))
	defer ts.Close()

	endpoint := resolvedEndpoint{
		url:           ts.URL,
		base:          ts.URL,
		method:        head,
		apiKey:        "irrelevant",
		limitRedirect: true,
	}

	client := &http.Client{}
	result, err := endpoint.checkHead(client)
	assert.NoError(t, err)
	assert.Equal(t, "Success", result)
}
