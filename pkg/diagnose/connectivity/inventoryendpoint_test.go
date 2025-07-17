// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestBuildRoute(t *testing.T) {
	tests := []struct {
		name           string
		prefix         string
		domain         domain
		path           string
		urlOverrideKey string
		versioned      bool
		expected       string
	}{
		{
			name:   "basic route with dot separator",
			prefix: "install",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:     "api/v1/validate",
			expected: "https://install.datadoghq.com./api/v1/validate",
		},
		{
			name:   "prefix already has separator",
			prefix: "llmobs-intake.",
			domain: domain{
				site:          "datadoghq.com",
				infraEndpoint: "https://app.datadoghq.com",
			},
			path:     "api/v2/llmobs",
			expected: "https://llmobs-intake.datadoghq.com./api/v2/llmobs",
		},
		{
			name:   "path with leading slash and custom site",
			prefix: "install",
			domain: domain{
				site:          "datadoghq.eu",
				infraEndpoint: "https://app.datadoghq.eu",
			},
			path:     "/api/v1/validate",
			expected: "https://install.datadoghq.eu./api/v1/validate",
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
			expected:  "https://6-0-0-app.agent.datadoghq.com/api/v1/validate",
		},
		{
			name:   "with url override",
			prefix: "intake.profile",
			domain: domain{
				site:          "datadoghq.eu",
				infraEndpoint: "https://app.datadoghq.eu",
			},
			urlOverrideKey: "dd_url",
			path:           "validate",
			expected:       "http://myproxy.com/validate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpointDescription := endpointDescription{
				prefix:            tt.prefix,
				routePath:         tt.path,
				versioned:         tt.versioned,
				altURLOverrideKey: tt.urlOverrideKey,
			}
			version.AgentVersion = "6.0.0"
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("site", tt.domain.site)
			mockConfig.SetWithoutSource("multi_region_failover.enabled", tt.domain.isFailover)
			mockConfig.SetWithoutSource("multi_region_failover.site", tt.domain.site)
			if tt.urlOverrideKey != "" {
				mockConfig.SetWithoutSource(tt.urlOverrideKey, "http://myproxy.com")
			}
			url := endpointDescription.buildRoute(mockConfig, tt.domain)
			assert.Equal(t, tt.expected, url)
		})
	}
}

func TestBuildEndpoints(t *testing.T) {
	tests := []struct {
		name                string
		endpointDescription endpointDescription
		config              map[string]string
		domains             []domain
		expectedEndpoints   []resolvedEndpoint
	}{
		{
			name: "endpoint with route",
			endpointDescription: endpointDescription{
				route:  "https://custom.endpoint.com",
				method: http.MethodHead,
			},
			domains: []domain{
				{
					site:          "datadoghq.com",
					defaultAPIKey: "api-key-1",
				},
				{
					site:          "datadoghq.eu",
					defaultAPIKey: "api-key-2",
					isFailover:    true,
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
				prefix:          "install",
				method:          http.MethodHead,
				handlesFailover: true,
			},
			domains: []domain{
				{
					site:          "datadoghq.com",
					defaultAPIKey: "api-key-1",
				},
				{
					site:          "datadoghq.eu",
					defaultAPIKey: "api-key-2",
					isFailover:    true,
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-1",
					url:    "https://install.datadoghq.com./",
				},
				{
					apiKey: "api-key-2",
					url:    "https://install.datadoghq.eu./",
				},
			},
		},
		{
			name: "endpoint with config prefix",
			endpointDescription: endpointDescription{
				prefix:          "ndm.metadata.",
				method:          http.MethodGet,
				configPrefix:    "service.metadata.",
				handlesFailover: true,
			},
			config: map[string]string{
				"service.metadata.api_key": "api-key-custom",
			},
			domains: []domain{
				{
					site:          "datadoghq.com",
					defaultAPIKey: "api-key-main",
					useAltAPIKey:  true,
				},
				{
					site:          "datadoghq.mrf.com",
					defaultAPIKey: "api-key-mrf",
					useAltAPIKey:  false,
					isFailover:    true,
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-custom",
					url:    "https://ndm.metadata.datadoghq.com./",
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
				prefix:       "ndm.metadata.",
				method:       http.MethodGet,
				configPrefix: "service.metadata.",
			},
			domains: []domain{
				{
					site:          "datadoghq.com",
					defaultAPIKey: "api-key-1",
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-1",
					url:    "https://ndm.metadata.datadoghq.com./",
				},
			},
		},
		{
			name: "route with url override key",
			endpointDescription: endpointDescription{
				name:              "install",
				route:             "https://install.datadoghq.com",
				method:            http.MethodHead,
				altURLOverrideKey: "installer.registry.url",
			},
			config: map[string]string{
				"installer.registry.url": "https://override.com",
			},
			domains: []domain{
				{
					site:          "datadoghq.com",
					defaultAPIKey: "api-key-1",
				},
			},
			expectedEndpoints: []resolvedEndpoint{
				{
					apiKey: "api-key-1",
					url:    "https://override.com",
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

			mockConfig.SetWithoutSource("site", tt.domains[0].site)
			if len(tt.domains) > 1 {
				mockConfig.SetWithoutSource("multi_region_failover.enabled", true)
				mockConfig.SetWithoutSource("multi_region_failover.site", tt.domains[1].site)
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
				assert.True(t, found, "Expected URL %s not found in endpoints - found %v", expectedEndpoint.url, endpoints)
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
		expectedKeys              []domain
	}{
		{
			name:     "main site",
			apiKey:   "test-api-key",
			site:     "datadOg.com",
			expected: "datadOg.com",
			expectedKeys: []domain{
				{
					site:          "datadOg.com",
					defaultAPIKey: "test-api-key",
					infraEndpoint: "https://app.datadOg.com",
					useAltAPIKey:  true,
				},
			},
		},
		{
			name:     "default site",
			apiKey:   "test-api-key",
			expected: "datadoghq.com",
			expectedKeys: []domain{
				{
					site:          "datadoghq.com",
					defaultAPIKey: "test-api-key",
					infraEndpoint: "https://app.datadoghq.com.",
					useAltAPIKey:  true,
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
			expectedKeys: []domain{
				{
					site:          "datadoghq.eu",
					defaultAPIKey: "test-api-key",
					infraEndpoint: "https://app.datadoghq.eu.",
					useAltAPIKey:  true,
				},
				{
					site:          "datadoghq.com",
					defaultAPIKey: "test-api-key",
					infraEndpoint: "https://app.datadoghq.com.",
					useAltAPIKey:  false,
					isFailover:    true,
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
		method: http.MethodGet,
		apiKey: "test-api-key",
	}

	// Create HTTP client
	client := &http.Client{}

	// Test successful GET request
	result, err := endpoint.checkGet(context.Background(), client)
	assert.NoError(t, err)
	assert.Equal(t, "Success", result)

	// Test with wrong API key
	endpoint.apiKey = "wrong-api-key"
	_, err = endpoint.checkGet(context.Background(), client)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status code: 401")
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
		method:        http.MethodHead,
		apiKey:        "irrelevant",
		limitRedirect: true,
	}

	client := &http.Client{}
	result, err := endpoint.checkHead(context.Background(), client)
	assert.NoError(t, err)
	assert.Equal(t, "Success", result)
}

func TestRun(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("api_key", "test-api-key")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else {
			if r.Header.Get("DD-API-KEY") == "test-api-key" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
			}
		}
	}))
	defer ts.Close()

	testEndpoints := []resolvedEndpoint{
		{
			url:    ts.URL,
			method: http.MethodHead,
		},
		{
			url:    ts.URL,
			method: http.MethodGet,
			apiKey: "test-api-key",
		},
		{
			url:    ts.URL,
			method: http.MethodGet,
			apiKey: "wrong-api-key",
		},
	}

	clientNormal := getClient(cfg, 2, logmock.New(t))
	clientRedirect := getClient(cfg, 2, logmock.New(t), withOneRedirect())

	diagnoses, err := checkEndpoints(context.Background(), testEndpoints, clientNormal, clientRedirect)
	assert.NoError(t, err)
	assert.Len(t, diagnoses, len(testEndpoints))
	successCount := 0
	failCount := 0
	for _, diagnosis := range diagnoses {
		if diagnosis.Status == diagnose.DiagnosisSuccess {
			successCount++
		} else {
			failCount++
		}
	}
	assert.Equal(t, 2, successCount)
	assert.Equal(t, 1, failCount)
}
