// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfig

import (
	"github.com/bazelbuild/rules_go/go/runfiles"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"strings"
	"testing"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"github.com/stretchr/testify/require"
)

// Exercise the actual Agent YAML reader, setup (including FIPS overrides), and
// endpoint consumers, with in-memory secret/delegated-auth mocks and dummy keys.
func loadRoutingConsumerConfig(t *testing.T, raw string) model.BuildableConfig {
	t.Helper()
	cfg := configmock.New(t)
	configmock.NewSystemProbe(t)
	path := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
	cfg.SetConfigFile(path)
	require.NoError(t, setup.LoadDatadog(cfg, secretsmock.New(t), delegatedauthmock.New(t), nil))
	return cfg
}

func TestGenerateWithRoutingPreservesAgentDictionaryLeaves(t *testing.T) {
	p, err := receivers.Capture("fakeintake", "http://fixture:8080", "")
	require.NoError(t, err)
	raw, err := GenerateWithRouting(p, "", `container_env_as_tags:
  TEAM: team
  APP: app
kubernetes_pod_labels_as_tags:
  tags.datadoghq.com/ENV: env
  example.com/team: team
  SimpleLabel: label
apm_config:
  additional_profile_tags:
    Build.ID: build-value
    OWNER: owner-value
tags: [keep:me, another:tag]
`)
	require.NoError(t, err)
	cfg := loadRoutingConsumerConfig(t, raw)
	require.Equal(t, map[string]string{"TEAM": "team", "APP": "app"}, cfg.GetStringMapString("container_env_as_tags"))
	require.Equal(t, map[string]string{"tags.datadoghq.com/ENV": "env", "example.com/team": "team", "SimpleLabel": "label"}, cfg.GetStringMapString("kubernetes_pod_labels_as_tags"))
	require.Equal(t, map[string]string{"Build.ID": "build-value", "OWNER": "owner-value"}, cfg.GetStringMapString("apm_config.additional_profile_tags"))
	require.Equal(t, []string{"keep:me", "another:tag"}, cfg.GetStringSlice("tags"))
}

func TestGenerateWithRoutingRuntimeEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://fixture:8080", "https://fixture:8443"} {
		t.Run(endpoint, func(t *testing.T) {
			p, err := receivers.Capture("blackhole", endpoint, "")
			require.NoError(t, err)
			raw, err := GenerateWithRouting(p, "", "")
			require.NoError(t, err)
			cfg := loadRoutingConsumerConfig(t, raw)
			require.False(t, cfg.GetBool("fips.enabled"))
			require.False(t, cfg.GetBool("multi_region_failover.enabled"))
			require.Equal(t, endpoint, cfg.GetString("dd_url"))
			destinations, err := configutils.GetMultipleEndpoints(cfg)
			require.NoError(t, err)
			require.Len(t, destinations, 1)
			destination, ok := destinations[endpoint]
			require.True(t, ok)
			require.False(t, destination.IsMRF)
			require.Equal(t, receivers.DummyAPIKey, cfg.GetString("api_key"))
		})
	}
}

func TestGenerateWithRoutingRejectsRuntimeDestinationOverrides(t *testing.T) {
	p, err := receivers.Capture("blackhole", "http://fixture:8080", "")
	require.NoError(t, err)
	// Demonstrate the real runtime precedence that makes these raw knobs unsafe,
	// then verify explicit rendering refuses them before setup can consume them.
	t.Run("FIPS", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "")
		t.Setenv("HTTPS_PROXY", "")
		extra := "fips: {enabled: true, https: false, local_address: '127.0.0.1', port_range_start: 5000}"
		cfg := loadRoutingConsumerConfig(t, "api_key: "+receivers.DummyAPIKey+"\ndd_url: http://fixture:8080\n"+extra)
		require.Equal(t, "http://127.0.0.1:5001", cfg.GetString("dd_url"))
		_, err := GenerateWithRouting(p, "", extra)
		require.Error(t, err)
	})
	t.Run("MRF", func(t *testing.T) {
		extra := "multi_region_failover: {enabled: true, failover_metrics: true, site: datadoghq.eu, api_key: dummy-secondary-key}"
		cfg := loadRoutingConsumerConfig(t, "api_key: "+receivers.DummyAPIKey+"\ndd_url: http://fixture:8080\n"+extra)
		destinations, err := configutils.GetMultipleEndpoints(cfg)
		require.NoError(t, err)
		require.Len(t, destinations, 2)
		_, err = GenerateWithRouting(p, "", extra)
		require.Error(t, err)
	})
}

// Shared fixture conformance keeps the private core-handler consumer tests
// independent of this module while preventing their checked-in inputs drifting
// away from what the real receiver renderer produces.
func TestReceiverRoutingTraceFixtureConformance(t *testing.T) {
	for _, tc := range []struct {
		scheme string
		port   string
	}{{"http", "8080"}, {"https", "8443"}} {
		t.Run(tc.scheme, func(t *testing.T) {
			name := "receiver_routing_" + tc.scheme + ".yaml"
			path := filepath.Join("..", "..", "..", "..", "..", "pkg", "trace", "api", name)
			if location := os.Getenv("RECEIVER_ROUTING_" + strings.ToUpper(tc.scheme) + "_FIXTURE"); location != "" {
				var err error
				path, err = runfiles.Rlocation(location)
				require.NoError(t, err)
			}
			fixture, err := os.ReadFile(path)
			require.NoError(t, err)
			p, err := receivers.Capture("fakeintake", tc.scheme+"://receiver.invalid:"+tc.port, "")
			require.NoError(t, err)
			raw, err := GenerateWithRouting(p, "", "hostname: receiver-test\napm_config.enabled: true\n")
			require.NoError(t, err)
			var expected, actual map[string]any
			require.NoError(t, yaml.Unmarshal(fixture, &expected))
			require.NoError(t, yaml.Unmarshal([]byte(raw), &actual))
			require.Equal(t, expected, actual, "update core fixtures only after reviewing the real handler/transport regressions")
		})
	}
}
