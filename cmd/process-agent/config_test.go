// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

func mkurl(rawurl string) *url.URL {
	urlResult, err := url.Parse(rawurl)
	if err != nil {
		panic(err)
	}
	return urlResult
}

func getCheckNames(checks []checks.Check) []string {
	names := make([]string, len(checks))
	for idx, ch := range checks {
		names[idx] = ch.Name()
	}
	return names
}

func assertContainsCheck(t *testing.T, checks []checks.Check, name string) {
	t.Helper()
	assert.Contains(t, getCheckNames(checks), name)
}

func assertNotContainsCheck(t *testing.T, checks []checks.Check, name string) {
	t.Helper()
	assert.NotContains(t, getCheckNames(checks), name)
}

func TestProcessDiscovery(t *testing.T) {
	scfg := &sysconfig.Config{}
	cfg := config.Mock(t)

	// Make sure the process_discovery check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", true)
		enabledChecks := getChecks(scfg, false)
		assertContainsCheck(t, enabledChecks, checks.DiscoveryCheckName)
	})

	// Make sure the process_discovery check can be disabled
	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", false)
		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.DiscoveryCheckName)
	})

	// Make sure the process and process_discovery checks are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", true)
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.DiscoveryCheckName)
	})
}

func TestContainerCheck(t *testing.T) {
	scfg := &sysconfig.Config{}
	cfg := config.Mock(t)

	// Make sure the container check can be enabled if the process check is disabled
	t.Run("containers enabled; rt enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", false)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessCheckName)
	})

	// Make sure that disabling RT disables the rt container check
	t.Run("containers enabled; rt disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", true)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
	})

	// Make sure the container check cannot be enabled if we cannot access containers
	t.Run("cannot access containers", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getChecks(scfg, false)
		assertNotContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
	})

	// Make sure the container and process check are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ProcessCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
	})
}

func TestProcessCheck(t *testing.T) {
	cfg := config.Mock(t)

	scfg, err := sysconfig.New("")
	assert.NoError(t, err)

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessCheckName)
	})

	// Make sure the process check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ProcessCheckName)
	})
}

func TestConnectionsCheck(t *testing.T) {
	syscfg := config.MockSystemProbe(t)
	syscfg.Set("system_probe_config.enabled", true)

	t.Run("enabled", func(t *testing.T) {
		syscfg.Set("network_config.enabled", true)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ConnectionsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		syscfg.Set("network_config.enabled", false)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.ConnectionsCheckName)
	})
}

func TestPodCheck(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{config.Kubernetes: {}})
	defer config.SetDetectedFeatures(nil)

	t.Run("enabled", func(t *testing.T) {
		// Resets the cluster name so that it isn't cached during the call to `getChecks()`
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", true)
		cfg.Set("cluster_name", "test")

		enabledChecks := getChecks(&sysconfig.Config{}, true)
		assertContainsCheck(t, enabledChecks, checks.PodCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", false)

		enabledChecks := getChecks(&sysconfig.Config{}, true)
		assertNotContainsCheck(t, enabledChecks, checks.PodCheckName)
	})
}

func TestProcessEventsCheck(t *testing.T) {
	scfg := &sysconfig.Config{}
	cfg := config.Mock(t)

	t.Run("default", func(t *testing.T) {
		enabledChecks := getChecks(scfg, false)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessEventsCheckName)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.event_collection.enabled", true)
		enabledChecks := getChecks(scfg, false)
		assertContainsCheck(t, enabledChecks, checks.ProcessEventsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.event_collection.enabled", false)
		enabledChecks := getChecks(scfg, false)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessEventsCheckName)
	})
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

			if eps, err := getAPIEndpoints(); tc.error {
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

			eps, err := getAPIEndpoints()
			assert.NoError(t, err)

			mainEndpoint := eps[0]
			assert.Equal(t, tc.expectedHostname, mainEndpoint.Endpoint.Hostname())

			eventsEps, err := getEventsAPIEndpoints()
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

			eps, err := getAPIEndpoints()
			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedEndpoints, eps)

			eventsEps, err := getEventsAPIEndpoints()
			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedEventsEndpoints, eventsEps)
		})
	}
}
