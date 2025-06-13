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

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
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
				prefix:    tt.prefix,
				path:      tt.path,
				separator: tt.separator,
				versioned: tt.versioned,
			}
			version.AgentVersion = "6.0.0"
			result := endpointDescription.buildRoute(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildEndpoints(t *testing.T) {
	tests := []struct {
		name                  string
		endpointDescription   endpointDescription
		mainEndpoint          string
		domains               map[string]domain
		expectedEndpointCount int
		expectedURLs          []string
	}{
		{
			name: "endpoint with route",
			endpointDescription: endpointDescription{
				route:  "https://custom.endpoint.com",
				method: head,
			},
			mainEndpoint: "datadoghq.com",
			domains: map[string]domain{
				"datadoghq.com": {
					site:   "datadoghq.com",
					apiKey: "api-key-1",
				},
			},
			expectedEndpointCount: 1,
			expectedURLs:          []string{"https://custom.endpoint.com"},
		},
		{
			name: "endpoint with prefix and multiple domains",
			endpointDescription: endpointDescription{
				prefix: "install",
				method: head,
			},
			mainEndpoint: "datadoghq.com",
			domains: map[string]domain{
				"main": {
					site:   "datadoghq.com",
					apiKey: "api-key-1",
				},
				"mrf": {
					site:   "datadoghq.eu",
					apiKey: "api-key-2",
				},
			},
			expectedEndpointCount: 2,
			expectedURLs: []string{
				"https://install.datadoghq.com/",
				"https://install.datadoghq.eu/",
			},
		},
		{
			name: "endpoint with prefix, path and separator",
			endpointDescription: endpointDescription{
				prefix:      "browser-intake",
				path:        "api/v2/logs",
				method:      post,
				contentType: json,
				separator:   dash,
			},
			mainEndpoint: "datadoghq.com",
			domains: map[string]domain{
				"datadoghq.com": {
					site:   "datadoghq.com",
					apiKey: "api-key-1",
				},
			},
			expectedEndpointCount: 1,
			expectedURLs:          []string{"https://browser-intake-datadoghq.com/api/v2/logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoints := tt.endpointDescription.buildEndpoints(tt.domains)

			assert.Len(t, endpoints, tt.expectedEndpointCount)

			for _, expectedURL := range tt.expectedURLs {
				found := false
				for _, endpoint := range endpoints {
					if expectedURL == endpoint.url {
						found = true
						assert.Equal(t, tt.endpointDescription.method, endpoint.method)
						assert.Equal(t, tt.endpointDescription.contentType, endpoint.contentType)
						break
					}
				}
				assert.True(t, found, "Expected URL %s not found in endpoints", expectedURL)
			}
		})
	}
}

func TestCheckServiceConnectivity(t *testing.T) {
	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" || r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	tests := []struct {
		name        string
		endpoint    endpoint
		expectError bool
	}{
		{
			name: "HEAD method success",
			endpoint: endpoint{
				url:    ts.URL,
				method: head,
			},
			expectError: false,
		},
		{
			name: "POST method success",
			endpoint: endpoint{
				url:    ts.URL,
				method: post,
			},
			expectError: false,
		},
		{
			name: "unknown method",
			endpoint: endpoint{
				url:    ts.URL,
				method: "INVALID", // Invalid method
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := checkServiceConnectivity(tt.endpoint)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown URL type")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckHead(t *testing.T) {
	// Test successful HEAD request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "HEAD", r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testEndpoint := endpoint{
		url:    ts.URL,
		method: head,
	}
	_, err := testEndpoint.checkHead()
	assert.NoError(t, err)

	// Test HEAD request with 404 response
	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts404.Close()

	testEndpoint = endpoint{
		url:    ts404.URL,
		method: head,
	}
	_, err = testEndpoint.checkHead()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status code: 404")
}

func TestCheckPost(t *testing.T) {
	// Test successful POST request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-api-key", r.Header.Get("DD-API-KEY"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testEndpoint := endpoint{
		url:    ts.URL,
		method: post,
		apiKey: "test-api-key",
	}

	_, err := testEndpoint.checkPost()
	assert.NoError(t, err)

	// Test POST request with 404 response
	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts404.Close()

	testEndpoint = endpoint{
		url:    ts404.URL,
		method: post,
		apiKey: "test-api-key",
	}

	_, err = testEndpoint.checkPost()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status code: 404")
}

func TestGetClient(t *testing.T) {
	client := getClient()
	assert.NotNil(t, client)
	assert.Equal(t, 5*time.Second, client.Timeout)

	// Test with custom options
	client = getClient(withOneRedirect())
	assert.NotNil(t, client)
	assert.NotNil(t, client.CheckRedirect)
}

func TestWithOneRedirect(t *testing.T) {
	client := &http.Client{}
	withOneRedirect()(client)
	assert.NotNil(t, client.CheckRedirect)
}

func TestDiagnoseDatadogURL(t *testing.T) {
	mockConfig := configmock.New(t)

	// Set required configuration
	mockConfig.SetWithoutSource("api_key", "test-api-key")
	mockConfig.SetWithoutSource("dd_url", "https://app.datadoghq.com")

	diagnoses := DiagnoseDatadogURL(mockConfig)
	assert.NotNil(t, diagnoses)
	assert.Greater(t, len(diagnoses), 0)

	// Check that we have diagnoses
	for _, diagnosis := range diagnoses {
		assert.NotEmpty(t, diagnosis.Name)
		assert.True(t, diagnosis.Status == diagnose.DiagnosisSuccess || diagnosis.Status == diagnose.DiagnosisFail)
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
					site:          "datadOg.com",
					apiKey:        "test-api-key",
					infraEndpoint: "https://app.datadOg.com",
				},
			},
		},
		{
			name:     "default site",
			apiKey:   "test-api-key",
			expected: "datadoghq.com",
			expectedKeys: map[string]domain{
				"main": {
					site:          "datadoghq.com",
					apiKey:        "test-api-key",
					infraEndpoint: "https://app.datadoghq.com.",
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
					site:          "datadoghq.eu",
					apiKey:        "test-api-key",
					infraEndpoint: "https://app.datadoghq.eu.",
				},
				"MRF": {
					site:          "datadoghq.com",
					apiKey:        "test-api-key",
					infraEndpoint: "https://app.mrf.datadoghq.com.",
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
			mockConfig.SetWithoutSource("multi_region_failover.apikey", tt.multiRegionFailoverAPIKey)
			domains := getDomains(mockConfig)
			assert.Equal(t, tt.expectedKeys, domains)
		})
	}
}
