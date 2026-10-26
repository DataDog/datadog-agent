// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestMetricsEndpointRoutingConfiguration(t *testing.T) {
	t.Setenv("DD_EXPERIMENTAL_METRICS_ENDPOINT_ROUTING_METRIC_PREFIX_ALLOWLIST", `{"https://secondary.example/Path": ["billing."]}`)
	cfg := configmock.NewFromYAML(t, `
site: us5.datadoghq.com
additional_endpoints:
  https://secondary.example/Path: [fake-key]
experimental:
  metrics_endpoint_routing:
    metric_allowlist:
      https://app.us5.datadoghq.com: [critical.slo]
      https://secondary.example/Path: []
`)
	routing, err := ReadMetricsEndpointRouting(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"critical.slo"}, routing.MetricAllowlist["https://app.us5.datadoghq.com"])
	require.Contains(t, routing.MetricAllowlist, "https://secondary.example/Path")
	require.Empty(t, routing.MetricAllowlist["https://secondary.example/Path"])
	require.Equal(t, []string{"billing."}, routing.MetricPrefixAllowlist["https://secondary.example/Path"])
	endpoints, err := GetMultipleEndpoints(cfg)
	require.NoError(t, err)
	require.Equal(t, "https://app.us5.datadoghq.com", routing.PrimaryEndpoint)
	require.Contains(t, endpoints, GetInfraEndpoint(cfg), "retain Agent transport normalization")
}

func TestMetricsEndpointRoutingRejectsAmbiguousNormalizedEndpoints(t *testing.T) {
	for _, site := range []string{"us5.datadoghq.com", "bücher.example"} {
		t.Run(site, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("site", site)
			cfg.SetInTest("convert_dd_site_fqdn.enabled", true)
			cfg.SetInTest("additional_endpoints", map[string][]string{GetInfraEndpoint(cfg): {"key"}})
			_, err := GetMultipleEndpoints(cfg)
			require.NoError(t, err, "preserve existing behavior without routing policies")
			cfg.SetInTest(MetricsEndpointRoutingKey+".metric_allowlist", map[string][]string{InfraURLPrefix + site: {"keep"}})
			_, err = GetMultipleEndpoints(cfg)
			require.ErrorContains(t, err, "set dd_url explicitly")
			cfg.SetInTest("dd_url", InfraURLPrefix+site)
			_, err = GetMultipleEndpoints(cfg)
			require.NoError(t, err)
		})
	}
}

func TestMetricsEndpointRoutingUnknownEndpoint(t *testing.T) {
	for _, key := range []string{"metric_allowlist", "metric_prefix_allowlist"} {
		t.Run(key, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("site", "us5.datadoghq.com")
			cfg.SetInTest("dd_url", "https://primary.example")
			cfg.SetInTest(MetricsEndpointRoutingKey+"."+key, map[string][]string{
				"https://app.us5.datadoghq.com": {},
			})
			_, err := GetMultipleEndpoints(cfg)
			require.ErrorContains(t, err, "matches neither the primary endpoint")
			cfg.SetInTest(MetricsEndpointRoutingKey+"."+key, map[string][]string{
				"https://primary.example": {},
			})
			_, err = GetMultipleEndpoints(cfg)
			require.NoError(t, err)
		})
	}
}

func TestMetricsEndpointRoutingInvalidTypes(t *testing.T) {
	for _, key := range []string{"metric_allowlist", "metric_prefix_allowlist"} {
		t.Run(key, func(t *testing.T) {
			for _, value := range []any{
				"not a map", []string{"not a map"},
				map[string]any{"https://app.datadoghq.com": "not a list"},
				map[string]any{"https://app.datadoghq.com": []int{42}},
				map[string]any{"https://app.datadoghq.com": []any{nil}},
				map[string]any{"https://app.datadoghq.com": nil},
				map[string]any{"https://app.datadoghq.com": []string(nil)},
				map[string]any{"https://app.datadoghq.com": []any(nil)},
			} {
				cfg := configmock.New(t)
				cfg.SetInTest(MetricsEndpointRoutingKey+"."+key, value)
				_, err := GetMultipleEndpoints(cfg)
				require.ErrorContains(t, err, MetricsEndpointRoutingKey+"."+key)
			}
		})
	}
}

func TestMetricsEndpointRoutingRequiresV2(t *testing.T) {
	for _, field := range []string{"metric_allowlist", "metric_prefix_allowlist"} {
		t.Run(field, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("use_v2_api.series", false)
			_, err := GetMultipleEndpoints(cfg)
			require.NoError(t, err)
			cfg.SetInTest(MetricsEndpointRoutingKey+"."+field, map[string][]string{"https://app.datadoghq.com": {}})
			_, err = GetMultipleEndpoints(cfg)
			require.ErrorContains(t, err, "requires use_v2_api.series")
		})
	}
}
