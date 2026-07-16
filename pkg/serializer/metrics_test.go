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
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	implzlib "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func TestBuildPipelines(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "true")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
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
		assert.True(t, conf.V3)
		assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
	}
}

func TestBuildPipelinesWithAdditionalEndpoints(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "true")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("additional_endpoints", map[string][]string{
		"http://example.test": {"another_key"},
		"http://another.test": {"test_key"},
	})

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		require.Len(t, ctx.Destinations, 2)
		assert.True(t, conf.V3)
		urls := []string{}
		for _, dest := range ctx.Destinations {
			assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
			urls = append(urls, dest.Resolver.GetBaseDomain())
		}
		assert.ElementsMatch(t, []string{"http://example.test", "http://another.test"}, urls)
	}
}

func TestBuildPipelinesWithAutoscalingFailover(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "true")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("autoscaling.failover.enabled", true)
	config.SetInTest("cluster_agent.enabled", true)
	config.SetInTest("cluster_agent.url", "https://cluster.agent.svc")
	config.SetInTest("cluster_agent.auth_token", "01234567890123456789012345678901")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]

		switch dest.Resolver.GetBaseDomain() {
		case "http://example.test":
			assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			assert.True(t, conf.V3)
			assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
		case "https://cluster.agent.svc":
			assert.ElementsMatch(t,
				config.GetStringSlice("autoscaling.failover.metrics"),
				conf.Filter.(metrics.MapFilter).ToList())
			assert.False(t, conf.V3)
			assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		default:
			t.Fatal("unexpected destination address")
		}
	}
}

// Autoscaling failover with no metrics configured does not send anything to the failover endpoint.
func TestBuildPipelinesWithAutoscalingFailoverEmptyList(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "true")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("autoscaling.failover.enabled", true)
	config.SetInTest("autoscaling.failover.metrics", []string{})
	config.SetInTest("cluster_agent.enabled", true)
	config.SetInTest("cluster_agent.url", "https://cluster.agent.svc")
	config.SetInTest("cluster_agent.auth_token", "01234567890123456789012345678901")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
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
		assert.True(t, conf.V3)
		assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
	}
}

func TestBuildPipelinesWithMRFInactive(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "true")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("multi_region_failover.enabled", true)
	config.SetInTest("multi_region_failover.dd_url", "http://mrf.example.test")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.True(t, conf.V3)
		assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)

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

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "true")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("multi_region_failover.enabled", true)
	config.SetInTest("multi_region_failover.failover_metrics", true)
	config.SetInTest("multi_region_failover.dd_url", "http://mrf.example.test")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 2)
		assert.True(t, conf.V3)
		for _, dest := range ctx.Destinations {
			assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)

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

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "true")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("multi_region_failover.enabled", true)
	config.SetInTest("multi_region_failover.failover_metrics", true)
	config.SetInTest("multi_region_failover.metric_allowlist", []string{"datadog.agent.running"})
	config.SetInTest("multi_region_failover.dd_url", "http://mrf.example.test")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.True(t, conf.V3)
		assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)

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

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("autoscaling.failover.enabled", true)
	config.SetInTest("cluster_agent.enabled", true)
	config.SetInTest("cluster_agent.url", "https://cluster.agent.svc")
	config.SetInTest("cluster_agent.auth_token", "01234567890123456789012345678901")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
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

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("use_v3_api.series.enabled", "false")
	config.SetInTest("additional_endpoints", map[string][]string{
		"http://example.test": {"alt_key"},
		// ensure protocol version setting works even when domain is rewritten by the forwarder
		"http://app.us5.datadoghq.com": {"test_key"},
	})
	config.SetInTest(
		"serializer_experimental_use_v3_api.series.endpoints",
		[]string{"http://example.test"})

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
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

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("use_v3_api.series.enabled", "false")
	config.SetInTest("additional_endpoints", map[string][]string{
		"http://example.test": {"alt_key"},
		// ensure protocol version setting works even when domain is rewritten by the forwarder
		"http://app.us5.datadoghq.com": {"test_key"},
	})
	config.SetInTest(
		"serializer_experimental_use_v3_api.series.endpoints",
		[]string{"http://app.us5.datadoghq.com"})

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
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

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("use_v3_api.series.enabled", "false")
	config.SetInTest("additional_endpoints", map[string][]string{
		"http://another.test": {"alt_key"},
	})
	config.SetInTest("serializer_experimental_use_v3_api.series.endpoints", []string{"http://example.test"})
	config.SetInTest("serializer_experimental_use_v3_api.series.validate", true)

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
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

func TestBuildPipelinesWithV3Beta(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("use_v3_api.series.enabled", "false")
	config.SetInTest("serializer_experimental_use_v3_api.series.endpoints", []string{"http://example.test"})
	config.SetInTest("serializer_experimental_use_v3_api.series.use_beta", true)

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		assert.True(t, conf.V3)
		assert.Equal(t, endpoints.V3BetaSeriesEndpoint, dest.Endpoint)
	}
}

func TestBuildPipelinesWithV3BetaCustomRoute(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("use_v3_api.series.enabled", "false")
	config.SetInTest("serializer_experimental_use_v3_api.series.endpoints", []string{"http://example.test"})
	config.SetInTest("serializer_experimental_use_v3_api.series.use_beta", true)
	config.SetInTest("serializer_experimental_use_v3_api.series.beta_route", "/api/intake/metrics/custom/series")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
		assert.True(t, conf.V3)
		assert.Equal(t, "/api/intake/metrics/custom/series", dest.Endpoint.Route)
		assert.Equal(t, endpoints.V3BetaSeriesEndpoint.Name, dest.Endpoint.Name)
	}
}

// fixedRand is a deterministic prng that returns a fixed value, used to make
// shadow-sampling decisions reproducible in tests.
type fixedRand struct{ v float64 }

func (f fixedRand) Float64() float64 { return f.v }

func TestBuildPipelinesShadowSampleRateZero(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	config.SetInTest("use_v3_api.series.enabled", "false")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSeries, fixedRand{v: 0})
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.False(t, conf.V3)
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		assert.Empty(t, dest.ValidationBatchID)
	}
}

func TestBuildPipelinesShadowFires(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0.5)
	config.SetInTest("use_v3_api.series.enabled", "false")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSeries, fixedRand{v: 0.4})

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
		// v2 (authoritative) pipeline carries the batchID for correlation.
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.False(t, conf.V3, "V3")
			require.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "https://app.datadoghq.com", dest.Resolver.GetConfigName())
			require.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
			require.Equal(t, batchID, dest.ValidationBatchID)
		},
		// v3beta shadow pipeline carries the same batchID.
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.True(t, conf.V3, "V3")
			require.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "https://app.datadoghq.com", dest.Resolver.GetConfigName())
			require.Equal(t, "/api/intake/metrics/v3beta/series", dest.Endpoint.Route)
			require.Equal(t, endpoints.V3BetaSeriesEndpoint.Name, dest.Endpoint.Name)
			require.Equal(t, batchID, dest.ValidationBatchID)
		},
	)
}

func TestBuildPipelinesShadowSkippedAboveRate(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0.5)
	config.SetInTest("use_v3_api.series.enabled", "false")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSeries, fixedRand{v: 0.6})
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.False(t, conf.V3)
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		assert.Empty(t, dest.ValidationBatchID)
	}
}

func TestBuildPipelinesShadowSkippedWhenV3Authoritative(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.endpoints", []string{"https://app.datadoghq.com"})
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 1)
	config.SetInTest("use_v3_api.series.enabled", "false")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSeries, fixedRand{v: 0})
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.True(t, conf.V3)
		assert.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
		assert.Empty(t, dest.ValidationBatchID, "shadow must not fire when v3 is authoritative")
	}
}

func TestBuildPipelinesShadowSkippedForSketches(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 1)
	config.SetInTest("use_v3_api.series.enabled", "false")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSketches, fixedRand{v: 0})
	require.Len(t, pipelines, 1)

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.False(t, conf.V3)
		assert.Equal(t, endpoints.SketchSeriesEndpoint, dest.Endpoint)
		assert.Empty(t, dest.ValidationBatchID)
	}
}

// TestBuildPipelinesShadowSkippedForNonShadowSite verifies that the v3beta
// shadow does not fire for resolvers whose site is not in shadow_sites
// (default: only US1).
func TestBuildPipelinesShadowSkippedForNonShadowSite(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.us3.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 1)
	config.SetInTest("use_v3_api.series.enabled", "false")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSeries, fixedRand{v: 0})
	require.Len(t, pipelines, 1, "non-US1 site must not produce a v3beta shadow pipeline")

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.False(t, conf.V3)
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		assert.Empty(t, dest.ValidationBatchID)
	}
}

// TestBuildPipelinesShadowSitesKnobOptsInNonUS1 verifies that adding a site to
// shadow_sites enables shadowing for that site, producing a v2 pipeline plus a
// correlated v3beta shadow pipeline.
func TestBuildPipelinesShadowSitesKnobOptsInNonUS1(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.us3.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 1)
	config.SetInTest("use_v3_api.series.enabled", "false")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sites", []string{"us3.datadoghq.com"})

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSeries, fixedRand{v: 0})
	require.Len(t, pipelines, 2, "us3 must shadow when included in shadow_sites")

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
		// v2 (authoritative) pipeline targets the us3 series endpoint and carries
		// the batchID for correlation.
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.False(t, conf.V3, "V3")
			require.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "https://app.us3.datadoghq.com", dest.Resolver.GetConfigName())
			require.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
			require.Equal(t, batchID, dest.ValidationBatchID)
		},
		// v3beta shadow pipeline targets the same us3 resolver and carries the
		// same batchID.
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.True(t, conf.V3, "V3")
			require.Equal(t, metrics.AllowAllFilter{}, conf.Filter)
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "https://app.us3.datadoghq.com", dest.Resolver.GetConfigName())
			require.Equal(t, "/api/intake/metrics/v3beta/series", dest.Endpoint.Route)
			require.Equal(t, endpoints.V3BetaSeriesEndpoint.Name, dest.Endpoint.Name)
			require.Equal(t, batchID, dest.ValidationBatchID)
		},
	)
}

// TestBuildPipelinesShadowSkippedWhenVectorConfigured verifies that when
// metrics are diverted to a vector/OPW endpoint, the v3beta shadow does not
// fire even though the resolver's base domain is in shadow_sites. The shadow
// gate resolves the v2 series endpoint, so a non-Datadog destination falls
// out of the allow list.
func TestBuildPipelinesShadowSkippedWhenVectorConfigured(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)

	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("vector.metrics.enabled", true)
	config.SetInTest("vector.metrics.url", "https://vector.example.test:8080")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 1)
	config.SetInTest("use_v3_api.series.enabled", "false")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	s := NewSerializer(f, nil, compressor, config, logger, "")

	pipelines := s.buildPipelinesRng(metricsKindSeries, fixedRand{v: 0})
	require.Len(t, pipelines, 1, "vector-diverted metrics must not produce a v3beta shadow pipeline")

	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		dest := ctx.Destinations[0]
		assert.False(t, conf.V3)
		assert.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		assert.Empty(t, dest.ValidationBatchID)
	}
}

// newSeriesV3Serializer builds a serializer with a deterministic v2 baseline (no shadow
// sampling) so the new use_v3_api.series tests can assert v2/v3 routing without the shadow
// path adding a second pipeline.
func newSeriesV3Serializer(t *testing.T, config model.BuildableConfig) *Serializer {
	t.Helper()
	logger := logmock.New(t)
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: config}).Comp
	return NewSerializer(f, nil, compressor, config, logger, "")
}

// TestSeriesV3ModeFalse covers the "fully opt out" case: setting enabled=false forces v2
// for every resolver, including Datadog destinations.
func TestSeriesV3ModeFalse(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "false")

	s := newSeriesV3Serializer(t, config)
	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)
	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		assert.False(t, conf.V3)
		assert.Equal(t, endpoints.SeriesEndpoint, ctx.Destinations[0].Endpoint)
	}
}

// TestSeriesV3ModeDatadogOnly covers the safety-net mode: v3 for resolvers that resolve to
// a Datadog URL, v2 for everything else. The Datadog resolver tracks its config URL
// verbatim, so we must rely on Resolve(SeriesEndpoint) (and ddURLRegexp) rather than the
// raw configName here.
func TestSeriesV3ModeDatadogOnly(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "datadog_only")
	config.SetInTest("additional_endpoints", map[string][]string{
		"http://my-proxy.example.test": {"alt_key"},
	})

	s := newSeriesV3Serializer(t, config)
	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	testutil.ElementsMatchFn(t, maps.All(pipelines),
		// Datadog URL uses v3 under datadog_only
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "https://app.datadoghq.com", dest.Resolver.GetConfigName())
			require.True(t, conf.V3, "Datadog URL must use v3 under datadog_only")
			require.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
		},
		// third-party URL stays on v2 under datadog_only
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "http://my-proxy.example.test", dest.Resolver.GetConfigName())
			require.False(t, conf.V3, "third-party URL must stay on v2 under datadog_only")
			require.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		},
	)
}

// TestSeriesV3PerURLOverride covers `use_v3_api.series.endpoints`: a specific URL can
// be pinned back to v2 without affecting other resolvers.
func TestSeriesV3PerURLOverride(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("additional_endpoints", map[string][]string{
		"http://third-party.example.test": {"alt_key"},
	})
	config.SetInTest("use_v3_api.series.endpoints", map[string]string{
		"http://third-party.example.test": "false",
	})

	s := newSeriesV3Serializer(t, config)
	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 2)

	testutil.ElementsMatchFn(t, maps.All(pipelines),
		// default datadog_only enables v3 for the unlisted Datadog URL
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "https://app.datadoghq.com", dest.Resolver.GetConfigName())
			require.True(t, conf.V3, "default datadog_only must enable v3 for Datadog URLs")
			require.Equal(t, endpoints.V3SeriesEndpoint, dest.Endpoint)
		},
		// per-URL override pins the third-party URL to v2
		func(t require.TestingT, conf metrics.PipelineConfig, ctx *metrics.PipelineContext) {
			require.Len(t, ctx.Destinations, 1)
			dest := ctx.Destinations[0]
			require.Equal(t, "http://third-party.example.test", dest.Resolver.GetConfigName())
			require.False(t, conf.V3, "per-URL override must pin this URL to v2")
			require.Equal(t, endpoints.SeriesEndpoint, dest.Endpoint)
		},
	)
}

// TestSeriesV3VectorDivertedDefaultsToV2 covers the vector/OPW short-circuit: even with
// the global default of v3, metrics diverted through vector.metrics.url must default to
// v2 unless the vector-specific opt-in is set.
func TestSeriesV3VectorDivertedDefaultsToV2(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("vector.metrics.enabled", true)
	config.SetInTest("vector.metrics.url", "http://vector.example.test:8080")

	s := newSeriesV3Serializer(t, config)
	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)
	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		assert.False(t, conf.V3)
		assert.Equal(t, endpoints.SeriesEndpoint, ctx.Destinations[0].Endpoint)
	}
}

// TestSeriesV3VectorOptIn covers the vector opt-in path: when the operator flips
// vector.metrics.use_v3_api.series=true, the vector destination starts receiving v3 payloads.
func TestSeriesV3VectorOptIn(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("vector.metrics.enabled", true)
	config.SetInTest("vector.metrics.url", "http://vector.example.test:8080")
	config.SetInTest("vector.metrics.use_v3_api.series", true)

	s := newSeriesV3Serializer(t, config)
	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)
	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		assert.True(t, conf.V3)
		assert.Equal(t, endpoints.V3SeriesEndpoint, ctx.Destinations[0].Endpoint)
	}
}

// TestSeriesV3ExperimentalListWins covers the transition-period guarantee from the design
// doc: customers who opted into v3 via the legacy serializer_experimental_use_v3_api.series.endpoints
// keep their opt-in even if the global enabled=false says v2. The experimental list is a
// hard "force v3" override for one release cycle.
func TestSeriesV3ExperimentalListWins(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("dd_url", "http://example.test")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("use_v3_api.series.enabled", "false")
	config.SetInTest("serializer_experimental_use_v3_api.series.endpoints", []string{"http://example.test"})

	s := newSeriesV3Serializer(t, config)
	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)
	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		assert.True(t, conf.V3, "experimental opt-in must keep working as a transition fallback")
		assert.Equal(t, endpoints.V3SeriesEndpoint, ctx.Destinations[0].Endpoint)
	}
}

// TestSeriesV3ForcedToV2WhenCompressorImplIsZlib covers the build-tag mismatch the old
// config-string check missed: in a zlib-only build, serializer_compressor_kind="zstd" still
// resolves to the zlib implementation (see pkg/util/compression/selector/zlib-no-zstd.go).
// The guard inspects the actual compressor's ContentEncoding, so series must drop to v2 even
// though the config string says zstd.
func TestSeriesV3ForcedToV2WhenCompressorImplIsZlib(t *testing.T) {
	logger := logmock.New(t)
	config := configmock.New(t)
	config.SetInTest("dd_url", "https://app.datadoghq.com")
	config.SetInTest("api_key", "test_key")
	config.SetInTest("serializer_experimental_use_v3_api.series.shadow_sample_rate", 0)
	// Config says zstd, but the injected compressor is zlib, as a zlib-only build would resolve.
	config.SetInTest("serializer_compressor_kind", "zstd")

	f, err := defaultforwarderimpl.NewTestForwarder(defaultforwarder.Params{}, config, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	s := NewSerializer(f, nil, implzlib.New(), config, logger, "")
	require.Equal(t, compression.ZlibEncoding, s.Strategy.ContentEncoding(), "test precondition: compressor must be zlib")

	pipelines := s.buildPipelines(metricsKindSeries)
	require.Len(t, pipelines, 1)
	for conf, ctx := range pipelines {
		require.Len(t, ctx.Destinations, 1)
		assert.False(t, conf.V3, "a zlib compressor must force series to v2 regardless of the config string")
		assert.Equal(t, endpoints.SeriesEndpoint, ctx.Destinations[0].Endpoint)
	}
}

// TestEvalSeriesV3 exercises the full string-acceptance surface of
// use_v3_api.series.enabled / endpoints[url] in one place. The Datadog and
// non-Datadog resolvers exist so the "datadog_only" branch is observable from
// both sides. Empty/unrecognised inputs fall back to v2 (false) and emit a
// warning.
func TestEvalSeriesV3(t *testing.T) {
	logger := logmock.New(t)
	dd, err := resolver.NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:   "https://app.datadoghq.com",
		APIKeySet: []utils.APIKeys{utils.NewAPIKeys("api_key", "k")},
	})
	require.NoError(t, err)
	other, err := resolver.NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:   "http://proxy.example.test",
		APIKeySet: []utils.APIKeys{utils.NewAPIKeys("api_key", "k")},
	})
	require.NoError(t, err)

	cases := []struct {
		in        string
		wantDD    bool
		wantOther bool
	}{
		{"true", true, true},
		{"TRUE", true, true},
		{"yes", true, true},
		{"on", true, true},
		{"1", true, true},
		{"t", true, true},
		{"false", false, false},
		{"off", false, false},
		{"0", false, false},
		{"f", false, false},
		{"no", false, false},
		{"datadog_only", true, false},
		{"Datadog_Only", true, false},
		{"  true  ", true, true},
		// Empty and unrecognised values fall back to v2.
		{"", false, false},
		{"bogus", false, false},
	}
	for _, tc := range cases {
		gotDD := evalSeriesV3("use_v3_api.series.enabled", tc.in, dd, logger)
		assert.Equal(t, tc.wantDD, gotDD, "dd value=%q", tc.in)

		gotOther := evalSeriesV3("use_v3_api.series.enabled", tc.in, other, logger)
		assert.Equal(t, tc.wantOther, gotOther, "other value=%q", tc.in)
	}
}
