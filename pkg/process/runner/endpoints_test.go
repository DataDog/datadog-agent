// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

func mkurl(rawurl string) *url.URL {
	urlResult, err := url.Parse(rawurl)
	if err != nil {
		panic(err)
	}
	return urlResult
}

func TestGetAPIEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, apiKey, ddURL string
		additionalEndpoints map[string][]string
		expected            []apicfg.Endpoint
		error               bool
	}{
		{
			name:   "default",
			apiKey: "test",
			expected: []apicfg.Endpoint{
				{
					APIKey:            "test",
					Endpoint:          mkurl(pkgconfigsetup.DefaultProcessEndpoint),
					ConfigSettingPath: "api_key",
				},
			},
		},
		{
			name:   "invalid dd_url",
			apiKey: "test",
			ddURL:  "http://[fe80::%31%25en0]/", // from https://go.dev/src/net/url/url_test.go
			error:  true,
		},
		{
			name:   "multiple eps",
			apiKey: "test",
			additionalEndpoints: map[string][]string{
				"https://mock.datadoghq.com": {
					"key1",
					"key2",
				},
				"https://mock2.datadoghq.com": {
					"key1",
					"key3",
				},
			},
			expected: []apicfg.Endpoint{
				{
					Endpoint:          mkurl(pkgconfigsetup.DefaultProcessEndpoint),
					APIKey:            "test",
					ConfigSettingPath: "api_key",
				},
				{
					Endpoint:          mkurl("https://mock.datadoghq.com"),
					APIKey:            "key1",
					ConfigSettingPath: "process_config.additional_endpoints",
				},
				{
					Endpoint:          mkurl("https://mock.datadoghq.com"),
					APIKey:            "key2",
					ConfigSettingPath: "process_config.additional_endpoints",
				},
				{
					Endpoint:          mkurl("https://mock2.datadoghq.com"),
					APIKey:            "key1",
					ConfigSettingPath: "process_config.additional_endpoints",
				},
				{
					Endpoint:          mkurl("https://mock2.datadoghq.com"),
					APIKey:            "key3",
					ConfigSettingPath: "process_config.additional_endpoints",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("api_key", tc.apiKey)
			if tc.ddURL != "" {
				cfg.SetWithoutSource("process_config.process_dd_url", tc.ddURL)
			}
			if tc.additionalEndpoints != nil {
				cfg.SetWithoutSource("process_config.additional_endpoints", tc.additionalEndpoints)
			}

			if eps, err := endpoint.GetAPIEndpoints(cfg); tc.error {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expected, eps)
			}
		})
	}
}

// TestGetAPIEndpointsSite is a test for GetAPIEndpoints. It makes sure that the deprecated `site` setting still works
func TestGetAPIEndpointsSite(t *testing.T) {
	for _, tc := range []struct {
		name             string
		site             string
		ddURL            string
		expectedHostname string
	}{
		{
			name:             "site only",
			site:             "datadoghq.io",
			expectedHostname: "process.datadoghq.io",
		},
		{
			name:             "dd_url only",
			ddURL:            "https://process.datadoghq.eu",
			expectedHostname: "process.datadoghq.eu",
		},
		{
			name:             "both site and dd_url",
			site:             "datacathq.eu",
			ddURL:            "https://burrito.com",
			expectedHostname: "burrito.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			if tc.site != "" {
				cfg.SetWithoutSource("site", tc.site)
			}
			if tc.ddURL != "" {
				cfg.SetWithoutSource("process_config.process_dd_url", tc.ddURL)
			}

			eps, err := endpoint.GetAPIEndpoints(cfg)
			assert.NoError(t, err)

			mainEndpoint := eps[0]
			assert.Equal(t, tc.expectedHostname, mainEndpoint.Endpoint.Hostname())
		})
	}
}

// TestGetConcurrentAPIEndpoints ensures that process endpoints can be independently set
func TestGetConcurrentAPIEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name                string
		ddURL, apiKey       string
		additionalEndpoints map[string][]string
		expectedEndpoints   []apicfg.Endpoint
	}{
		{
			name:   "default",
			apiKey: "test",
			expectedEndpoints: []apicfg.Endpoint{
				{
					APIKey:            "test",
					Endpoint:          mkurl(pkgconfigsetup.DefaultProcessEndpoint),
					ConfigSettingPath: "api_key",
				},
			},
		},
		{
			name:   "set only process endpoint",
			ddURL:  "https://process.datadoghq.eu",
			apiKey: "test",
			expectedEndpoints: []apicfg.Endpoint{
				{
					APIKey:            "test",
					Endpoint:          mkurl("https://process.datadoghq.eu"),
					ConfigSettingPath: "api_key",
				},
			},
		},
		{
			name:   "multiple eps",
			apiKey: "test",
			additionalEndpoints: map[string][]string{
				"https://mock.datadoghq.com": {
					"key1",
					"key2",
				},
				"https://mock2.datadoghq.com": {
					"key3",
				},
			},
			expectedEndpoints: []apicfg.Endpoint{
				{
					Endpoint:          mkurl(pkgconfigsetup.DefaultProcessEndpoint),
					APIKey:            "test",
					ConfigSettingPath: "api_key",
				},
				{
					Endpoint:          mkurl("https://mock.datadoghq.com"),
					APIKey:            "key1",
					ConfigSettingPath: "process_config.additional_endpoints",
				},
				{
					Endpoint:          mkurl("https://mock.datadoghq.com"),
					APIKey:            "key2",
					ConfigSettingPath: "process_config.additional_endpoints",
				},
				{
					Endpoint:          mkurl("https://mock2.datadoghq.com"),
					APIKey:            "key3",
					ConfigSettingPath: "process_config.additional_endpoints",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("api_key", tc.apiKey)
			if tc.ddURL != "" {
				cfg.SetWithoutSource("process_config.process_dd_url", tc.ddURL)
			}

			if tc.additionalEndpoints != nil {
				cfg.SetWithoutSource("process_config.additional_endpoints", tc.additionalEndpoints)
			}

			eps, err := endpoint.GetAPIEndpoints(cfg)
			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedEndpoints, eps)
		})
	}
}
