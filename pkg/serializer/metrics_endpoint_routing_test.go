// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestEndpointMetricFilterGrouping(t *testing.T) {
	filters := compileEndpointMetricFilters(configutils.MetricsEndpointRouting{
		MetricAllowlist:       map[string][]string{"a": {"critical.slo", "billing.latency", "critical.slo"}, "b": {"critical.slo"}},
		MetricPrefixAllowlist: map[string][]string{"a": {"billing.latency", "billing.", "billing."}, "b": {"billing."}, "all": {""}},
	})
	require.Same(t, filters["a"], filters["b"])
	require.True(t, filters["all"].Filter(&metrics.Serie{Name: "any.metric"}))
}

func TestEndpointMetricRoutingSitePrimaryWithMetricsOverride(t *testing.T) {
	for _, prefix := range []string{"vector", "observability_pipelines_worker"} {
		t.Run(prefix, func(t *testing.T) {
			const primary = "https://app.us5.datadoghq.com"
			cfg := configmock.New(t)
			cfg.SetInTest("site", "us5.datadoghq.com")
			cfg.SetInTest("api_key", "fake-key")
			cfg.SetInTest(prefix+".metrics.enabled", true)
			cfg.SetInTest(prefix+".metrics.url", "http://worker.example")
			cfg.SetInTest(configutils.MetricsEndpointRoutingKey+".metric_allowlist", map[string][]string{primary: {"keep"}})
			s := newSeriesV3Serializer(t, cfg)
			pipelines := s.buildPipelines(metricsKindSeries)
			require.Len(t, pipelines, 1)
			for conf, context := range pipelines {
				require.Len(t, context.Destinations, 1)
				destination := context.Destinations[0]
				require.Equal(t, configutils.GetInfraEndpoint(cfg), destination.Resolver.GetConfigName())
				require.Equal(t, "http://worker.example", destination.Resolver.Resolve(destination.Endpoint))
				require.True(t, conf.Filter.Filter(&metrics.Serie{Name: "keep"}))
				require.False(t, conf.Filter.Filter(&metrics.Serie{Name: "drop"}))
			}
		})
	}
}
