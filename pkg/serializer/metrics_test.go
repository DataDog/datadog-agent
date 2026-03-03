// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func TestBuildPipelines(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		assert.Equal(t, "http://example.test", dest.Resolver.GetBaseDomain())
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
	}
}

func TestBuildPipelinesWithAdditionalEndpoints(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("additional_endpoints", map[string][]string{
		"http://example.test": {"another_key"},
		"http://another.test": {"test_key"},
	})

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		require.Len(t, ctx.Destinations, 2)
		urls := []string{}
		for _, dest := range ctx.Destinations {
			assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
			urls = append(urls, dest.Resolver.GetBaseDomain())
		}
		assert.ElementsMatch(t, []string{"http://example.test", "http://another.test"}, urls)
	}
}

func TestBuildPipelinesWithAutoscalingFailover(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("autoscaling.failover.enabled", true)
	config.SetWithoutSource("cluster_agent.enabled", true)
	config.SetWithoutSource("cluster_agent.url", "https://cluster.agent.svc")
	config.SetWithoutSource("cluster_agent.auth_token", "01234567890123456789012345678901")

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)

		switch dest.Resolver.GetBaseDomain() {
		case "http://example.test":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		case "https://cluster.agent.svc":
			assert.ElementsMatch(t,
				config.GetStringSlice("autoscaling.failover.metrics"),
				conf.Filter.(metrics.MapFilter).ToList())
		default:
			t.Fatal("unexpected destination address")
		}
	}
}

// Autoscaling failover with no metrics configured does not send anything to the failover endpoint.
func TestBuildPipelinesWithAutoscalingFailoverEmptyList(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("autoscaling.failover.enabled", true)
	config.SetWithoutSource("autoscaling.failover.metrics", []string{})
	config.SetWithoutSource("cluster_agent.enabled", true)
	config.SetWithoutSource("cluster_agent.url", "https://cluster.agent.svc")
	config.SetWithoutSource("cluster_agent.auth_token", "01234567890123456789012345678901")

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		assert.Equal(t, "http://example.test", dest.Resolver.GetBaseDomain())
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
	}
}

func TestBuildPipelinesWithMRFInactive(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("multi_region_failover.enabled", true)
	config.SetWithoutSource("multi_region_failover.dd_url", "http://mrf.example.test")

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)

		switch dest.Resolver.GetBaseDomain() {
		case "http://example.test":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		default:
			t.Fatal("unexpected destination address")
		}
	}
}

func TestBuildPipelinesWithMRFActive(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("multi_region_failover.enabled", true)
	config.SetWithoutSource("multi_region_failover.failover_metrics", true)
	config.SetWithoutSource("multi_region_failover.dd_url", "http://mrf.example.test")

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 2)
		for _, dest := range ctx.Destinations {
			assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)

			switch dest.Resolver.GetBaseDomain() {
			case "http://example.test":
				assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			case "http://mrf.example.test":
				assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			default:
				t.Fatal("unexpected destination address")
			}
		}
	}
}

func TestBuildPipelinesWithMRFActiveFilter(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("multi_region_failover.enabled", true)
	config.SetWithoutSource("multi_region_failover.failover_metrics", true)
	config.SetWithoutSource("multi_region_failover.metric_allowlist", []string{"datadog.agent.running"})
	config.SetWithoutSource("multi_region_failover.dd_url", "http://mrf.example.test")

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)

		switch dest.Resolver.GetBaseDomain() {
		case "http://example.test":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		case "http://mrf.example.test":
			assert.Equal(t,
				config.GetStringSlice("multi_region_failover.metric_allowlist"),
				conf.Filter.(metrics.MapFilter).ToList())
		default:
			t.Fatal("unexpected destination address")
		}
	}
}

func TestBuildPipelinesSketches(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("autoscaling.failover.enabled", true)
	config.SetWithoutSource("cluster_agent.enabled", true)
	config.SetWithoutSource("cluster_agent.url", "https://cluster.agent.svc")
	config.SetWithoutSource("cluster_agent.auth_token", "01234567890123456789012345678901")

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSketches)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		assert.Equal(t, "http://example.test", dest.Resolver.GetBaseDomain())
		assert.Equal(t, endpoints.SketchSeriesEndpoint, dest.Endpoint)
	}
}

func TestPipelinesWithV3AndAdditionalEndpoints(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("additional_endpoints", map[string][]string{
		"http://example.test": {"alt_key"},
		// ensure protocol version setting works even when domain is rewritten by the forwarder
		"http://app.us5.datadoghq.com": {"test_key"},
	})
	config.SetWithoutSource(
		"serializer_experimental_use_v3_api.series.endpoints",
		[]string{"http://example.test"})

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]

		switch dest.Resolver.GetConfigName() {
		case "http://example.test":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			assert.True(t, conf.V3)
			assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
		case "http://app.us5.datadoghq.com":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			assert.False(t, conf.V3)
			assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		default:
			t.Fatal("unknown destination address")
		}
	}
}

func TestPipelinesWithAdditionalEndpointsV3(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("additional_endpoints", map[string][]string{
		"http://example.test": {"alt_key"},
		// ensure protocol version setting works even when domain is rewritten by the forwarder
		"http://app.us5.datadoghq.com": {"test_key"},
	})
	config.SetWithoutSource(
		"serializer_experimental_use_v3_api.series.endpoints",
		[]string{"http://app.us5.datadoghq.com"})

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]

		switch dest.Resolver.GetConfigName() {
		case "http://example.test":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			assert.False(t, conf.V3)
			assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		case "http://app.us5.datadoghq.com":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			assert.True(t, conf.V3)
			assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
		default:
			t.Fatal("unknown destination address")
		}
	}
}

func TestPipelinesWithV3Validate(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetWithoutSource("dd_url", "http://example.test")
	config.SetWithoutSource("api_key", "test_key")
	config.SetWithoutSource("additional_endpoints", map[string][]string{
		"http://another.test": {"alt_key"},
	})
	config.SetWithoutSource("serializer_experimental_use_v3_api.series.endpoints", []string{"http://example.test"})
	config.SetWithoutSource("serializer_experimental_use_v3_api.series.validate", true)

	f, err := defaultforwarder.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)

	batchID := ""
outer:
	for _, ctx := range pipelines {
		for _, d := range ctx.Destinations {
			if d.ValidationBatchID != "" {
				batchID = d.ValidationBatchID
				break outer
			}
		}
	}
	require.NotEmpty(t, batchID)

	testutil.ElementsMatchFn(t, maps.All(pipelines),
		// v3 pipeline has one destination...
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.True(t, conf.V3, "V3")
			require.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			testutil.ElementsMatchFn(t, slices.All(ctx.Destinations),
				// ... to the default domain with validation headers
				func(t require.TestingT, _ int, dest metrics.PipelineDestination) {
					require.Equal(t, "http://example.test", dest.Resolver.GetConfigName())
					require.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
					require.Equal(t, batchID, dest.ValidationBatchID)
				})
		},
		// v2 pipeline has two destinations...
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.False(t, conf.V3, "V3")
			require.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			testutil.ElementsMatchFn(t, slices.All(ctx.Destinations),
				// ... to the default domain with validation headers
				func(t require.TestingT, _ int, dest metrics.PipelineDestination) {
					require.Equal(t, "http://example.test", dest.Resolver.GetConfigName())
					require.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
					require.Equal(t, batchID, dest.ValidationBatchID)
				},
				// ... to the alternative domain without validation headers
				func(t require.TestingT, _ int, dest metrics.PipelineDestination) {
					require.Equal(t, "http://another.test", dest.Resolver.GetConfigName())
					require.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
					require.Empty(t, dest.ValidationBatchID)
				},
			)
		},
	)
}
