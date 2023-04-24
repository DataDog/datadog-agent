// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
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
					APIKey:   "test",
					Endpoint: mkurl(config.DefaultProcessEndpoint),
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
					Endpoint: mkurl(config.DefaultProcessEndpoint),
					APIKey:   "test",
				},
				{
					Endpoint: mkurl("https://mock.datadoghq.com"),
					APIKey:   "key1",
				},
				{
					Endpoint: mkurl("https://mock.datadoghq.com"),
					APIKey:   "key2",
				},
				{
					Endpoint: mkurl("https://mock2.datadoghq.com"),
					APIKey:   "key1",
				},
				{
					Endpoint: mkurl("https://mock2.datadoghq.com"),
					APIKey:   "key3",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Mock(t)
			cfg.Set("api_key", tc.apiKey)
			if tc.ddURL != "" {
				cfg.Set("process_config.process_dd_url", tc.ddURL)
			}
			if tc.additionalEndpoints != nil {
				cfg.Set("process_config.additional_endpoints", tc.additionalEndpoints)
			}

			if eps, err := GetAPIEndpoints(cfg); tc.error {
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
		name                                     string
		site                                     string
		ddURL, eventsDDURL                       string
		expectedHostname, expectedEventsHostname string
	}{
		{
			name:                   "site only",
			site:                   "datadoghq.io",
			expectedHostname:       "process.datadoghq.io",
			expectedEventsHostname: "process-events.datadoghq.io",
		},
		{
			name:                   "dd_url only",
			ddURL:                  "https://process.datadoghq.eu",
			expectedHostname:       "process.datadoghq.eu",
			expectedEventsHostname: "process-events.datadoghq.com",
		},
		{
			name:                   "events_dd_url only",
			eventsDDURL:            "https://process-events.datadoghq.eu",
			expectedHostname:       "process.datadoghq.com",
			expectedEventsHostname: "process-events.datadoghq.eu",
		},
		{
			name:                   "both site and dd_url",
			site:                   "datacathq.eu",
			ddURL:                  "https://burrito.com",
			eventsDDURL:            "https://burrito-events.com",
			expectedHostname:       "burrito.com",
			expectedEventsHostname: "burrito-events.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Mock(t)
			if tc.site != "" {
				cfg.Set("site", tc.site)
			}
			if tc.ddURL != "" {
				cfg.Set("process_config.process_dd_url", tc.ddURL)
			}
			if tc.eventsDDURL != "" {
				cfg.Set("process_config.events_dd_url", tc.eventsDDURL)
			}

			eps, err := GetAPIEndpoints(cfg)
			assert.NoError(t, err)

			mainEndpoint := eps[0]
			assert.Equal(t, tc.expectedHostname, mainEndpoint.Endpoint.Hostname())

			eventsEps, err := getEventsAPIEndpoints(cfg)
			assert.NoError(t, err)

			mainEventEndpoint := eventsEps[0]
			assert.Equal(t, tc.expectedEventsHostname, mainEventEndpoint.Endpoint.Hostname())
		})
	}
}

// TestGetConcurrentAPIEndpoints ensures that process and process-events endpoints can be independently set
func TestGetConcurrentAPIEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		ddURL, eventsDDURL, apiKey string
		additionalEndpoints        map[string][]string
		additionalEventsEndpoints  map[string][]string
		expectedEndpoints          []apicfg.Endpoint
		expectedEventsEndpoints    []apicfg.Endpoint
	}{
		{
			name:   "default",
			apiKey: "test",
			expectedEndpoints: []apicfg.Endpoint{
				{
					APIKey:   "test",
					Endpoint: mkurl(config.DefaultProcessEndpoint),
				},
			},
			expectedEventsEndpoints: []apicfg.Endpoint{
				{
					APIKey:   "test",
					Endpoint: mkurl(config.DefaultProcessEventsEndpoint),
				},
			},
		},
		{
			name:   "set only process endpoint",
			ddURL:  "https://process.datadoghq.eu",
			apiKey: "test",
			expectedEndpoints: []apicfg.Endpoint{
				{
					APIKey:   "test",
					Endpoint: mkurl("https://process.datadoghq.eu"),
				},
			},
			expectedEventsEndpoints: []apicfg.Endpoint{
				{
					APIKey:   "test",
					Endpoint: mkurl(config.DefaultProcessEventsEndpoint),
				},
			},
		},
		{
			name:        "set only process-events endpoint",
			eventsDDURL: "https://process-events.datadoghq.eu",
			apiKey:      "test",
			expectedEndpoints: []apicfg.Endpoint{
				{
					APIKey:   "test",
					Endpoint: mkurl(config.DefaultProcessEndpoint),
				},
			},
			expectedEventsEndpoints: []apicfg.Endpoint{
				{
					APIKey:   "test",
					Endpoint: mkurl("https://process-events.datadoghq.eu"),
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
			additionalEventsEndpoints: map[string][]string{
				"https://mock-events.datadoghq.com": {
					"key2",
				},
				"https://mock2-events.datadoghq.com": {
					"key3",
				},
			},
			expectedEndpoints: []apicfg.Endpoint{
				{
					Endpoint: mkurl(config.DefaultProcessEndpoint),
					APIKey:   "test",
				},
				{
					Endpoint: mkurl("https://mock.datadoghq.com"),
					APIKey:   "key1",
				},
				{
					Endpoint: mkurl("https://mock.datadoghq.com"),
					APIKey:   "key2",
				},
				{
					Endpoint: mkurl("https://mock2.datadoghq.com"),
					APIKey:   "key3",
				},
			},
			expectedEventsEndpoints: []apicfg.Endpoint{
				{
					Endpoint: mkurl(config.DefaultProcessEventsEndpoint),
					APIKey:   "test",
				},
				{
					Endpoint: mkurl("https://mock-events.datadoghq.com"),
					APIKey:   "key2",
				},
				{
					Endpoint: mkurl("https://mock2-events.datadoghq.com"),
					APIKey:   "key3",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Mock(t)
			cfg.Set("api_key", tc.apiKey)
			if tc.ddURL != "" {
				cfg.Set("process_config.process_dd_url", tc.ddURL)
			}

			if tc.eventsDDURL != "" {
				cfg.Set("process_config.events_dd_url", tc.eventsDDURL)
			}

			if tc.additionalEndpoints != nil {
				cfg.Set("process_config.additional_endpoints", tc.additionalEndpoints)
			}

			if tc.additionalEventsEndpoints != nil {
				cfg.Set("process_config.events_additional_endpoints", tc.additionalEventsEndpoints)
			}

			eps, err := GetAPIEndpoints(cfg)
			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedEndpoints, eps)

			eventsEps, err := getEventsAPIEndpoints(cfg)
			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedEventsEndpoints, eventsEps)
		})
	}
}
