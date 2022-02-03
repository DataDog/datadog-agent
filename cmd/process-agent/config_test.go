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
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

func TestProcessDiscovery(t *testing.T) {
	scfg, ocfg := &sysconfig.Config{}, &oconfig.OrchestratorConfig{}
	cfg := config.Mock()

	// Make sure the process_discovery check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", true)
		enabledChecks := getChecks(scfg, ocfg, false)
		assert.Contains(t, enabledChecks, checks.ProcessDiscovery)
	})

	// Make sure the process_discovery check can be disabled
	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", false)
		enabledChecks := getChecks(scfg, ocfg, true)
		assert.NotContains(t, enabledChecks, checks.ProcessDiscovery)
	})

	// Make sure the process and process_discovery checks are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", true)
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getChecks(scfg, ocfg, true)
		assert.NotContains(t, enabledChecks, checks.ProcessDiscovery)
	})
}

func TestContainerCheck(t *testing.T) {
	scfg, ocfg := &sysconfig.Config{}, &oconfig.OrchestratorConfig{}
	cfg := config.Mock()

	// Make sure the container check can be enabled if the process check is disabled
	t.Run("containers enabled; rt enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", false)

		enabledChecks := getChecks(scfg, ocfg, true)
		assert.Contains(t, enabledChecks, checks.Container)
		assert.Contains(t, enabledChecks, checks.RTContainer)
		assert.NotContains(t, enabledChecks, checks.Process)
	})

	// Make sure that disabling RT disables the rt container check
	t.Run("containers enabled; rt disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", true)

		enabledChecks := getChecks(scfg, ocfg, true)
		assert.Contains(t, enabledChecks, checks.Container)
		assert.NotContains(t, enabledChecks, checks.RTContainer)
	})

	// Make sure the container check cannot be enabled if we cannot access containers
	t.Run("cannot access containers", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getChecks(scfg, ocfg, false)
		assert.NotContains(t, enabledChecks, checks.Container)
		assert.NotContains(t, enabledChecks, checks.RTContainer)
	})

	// Make sure the container and process check are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getChecks(scfg, ocfg, true)
		assert.Contains(t, enabledChecks, checks.Process)
		assert.NotContains(t, enabledChecks, checks.Container)
		assert.NotContains(t, enabledChecks, checks.RTContainer)
	})
}

func TestProcessCheck(t *testing.T) {
	cfg := config.Mock()

	scfg, err := sysconfig.New("")
	assert.NoError(t, err)

	ocfg := &oconfig.OrchestratorConfig{}

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		enabledChecks := getChecks(scfg, ocfg, true)
		assert.NotContains(t, enabledChecks, checks.Process)
	})

	// Make sure the process check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getChecks(scfg, ocfg, true)
		assert.Contains(t, enabledChecks, checks.Process)
	})
}

func TestSysprobeProcessModule(t *testing.T) {
	cfg, ocfg := config.Mock(), &oconfig.OrchestratorConfig{}
	cfg.Set("process_config.process_collection.enabled", true)
	cfg.Set("system_probe_config.enabled", true)

	t.Run("enabled", func(t *testing.T) {
		cfg.Set("system_probe_config.process_config.enabled", true)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		_ = getChecks(scfg, ocfg, true)
		assert.True(t, checks.Process.SysprobeProcessModuleEnabled)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("system_probe_config.process_config.enabled", false)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		_ = getChecks(scfg, ocfg, false)
		assert.False(t, checks.Process.SysprobeProcessModuleEnabled)
	})
}

func TestConnectionsCheck(t *testing.T) {
	cfg := config.Mock()
	ocfg := &oconfig.OrchestratorConfig{}
	cfg.Set("system_probe_config.enabled", true)

	t.Run("enabled", func(t *testing.T) {
		cfg.Set("network_config.enabled", true)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getChecks(scfg, ocfg, true)
		assert.Contains(t, enabledChecks, checks.Connections)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("network_config.enabled", false)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getChecks(scfg, ocfg, true)
		assert.NotContains(t, enabledChecks, checks.Connections)
	})
}

func TestPodCheck(t *testing.T) {
	cfg := config.Mock()

	t.Run("enabled", func(t *testing.T) {
		cfg.Set("orchestrator_explorer.enabled", true)

		ocfg := oconfig.NewDefaultOrchestratorConfig()
		ocfg.KubeClusterName = "test" // We can't reliably detect a kubernetes cluster in a test
		assert.NoError(t, ocfg.Load())

		enabledChecks := getChecks(&sysconfig.Config{}, ocfg, true)
		assert.Contains(t, enabledChecks, checks.Pod)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("orchestrator_explorer.enabled", false)

		ocfg := oconfig.NewDefaultOrchestratorConfig()
		ocfg.KubeClusterName = "test" // We can't reliably detect a kubernetes cluster in a test
		assert.NoError(t, ocfg.Load())

		enabledChecks := getChecks(&sysconfig.Config{}, ocfg, true)
		assert.NotContains(t, enabledChecks, checks.Pod)
	})
}

func TestGetAPIEndpoints(t *testing.T) {
	mkurl := func(rawurl string) *url.URL {
		urlResult, err := url.Parse(rawurl)
		if err != nil {
			panic(err)
		}
		return urlResult
	}

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
			cfg := config.Mock()
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
				assert.EqualValues(t, tc.expected, eps)
			}
		})
	}
}

// TestGetAPIEndpointsSite is a test for GetAPIEndpoints. It makes sure that the deprecated `site` setting still works
func TestGetAPIEndpointsSite(t *testing.T) {
	for _, tc := range []struct {
		name, site, ddUrl, expectedHostname string
	}{
		{
			name:             "site only",
			site:             "datadoghq.io",
			expectedHostname: "process.datadoghq.io",
		},
		{
			name:             "dd_url only",
			ddUrl:            "https://process.datadoghq.eu",
			expectedHostname: "process.datadoghq.eu",
		},
		{
			name:             "both site and dd_url",
			site:             "datacathq.eu",
			ddUrl:            "https://burrito.com",
			expectedHostname: "burrito.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Mock()
			if tc.site != "" {
				cfg.Set("site", tc.site)
			}
			if tc.ddUrl != "" {
				cfg.Set("process_config.process_dd_url", tc.ddUrl)
			}

			eps, err := getAPIEndpoints()
			assert.NoError(t, err)

			mainEndpoint := eps[0]
			assert.Equal(t, tc.expectedHostname, mainEndpoint.Endpoint.Hostname())
		})
	}
}
