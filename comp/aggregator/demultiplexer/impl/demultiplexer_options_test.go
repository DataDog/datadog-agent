// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerimpl

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config/metricresolution"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func requireOptions(t *testing.T, cfg configmock.Component, params Params, factory aggregator.DogStatsDLookbackFactory) aggregator.AgentDemultiplexerOptions {
	t.Helper()
	options, err := createAgentDemultiplexerOptions(cfg, params, factory)
	require.NoError(t, err)
	return options
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountNotReadWithoutConfigOption(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               true,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := requireOptions(t, cfg, NewDefaultParams(), nil)
	require.Equal(t, 0, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountFromConfig(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               true,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := requireOptions(t, cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)
	require.Equal(t, 4, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountDefaultsToOneWhenEnabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{"dogstatsd_no_aggregation_pipeline": true})

	options := requireOptions(t, cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)
	require.Equal(t, 1, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountDisabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               false,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := requireOptions(t, cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)
	require.Equal(t, 0, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountFallsBackToOne(t *testing.T) {
	for _, configured := range []int{0, -2} {
		t.Run(fmt.Sprintf("configured_%d", configured), func(t *testing.T) {
			cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
				"dogstatsd_no_aggregation_pipeline":               true,
				"dogstatsd_no_aggregation_pipeline_workers_count": configured,
			})

			options := requireOptions(t, cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)
			require.Equal(t, 1, options.NoAggregationPipelineWorkersCount)
		})
	}
}

func TestCreateAgentDemultiplexerOptionsStoresLookbackFactory(t *testing.T) {
	cfg := configmock.NewMock(t)
	factory := aggregator.DogStatsDLookbackFactory(func(serializer.MetricSerializer) aggregator.DogStatsDLookback { return nil })

	options := requireOptions(t, cfg, NewDefaultParams(), factory)
	require.NotNil(t, options.DogStatsDLookbackFactory)
}

func TestCreateAgentDemultiplexerOptionsMetricResolutionExperimentDisabledParity(t *testing.T) {
	t.Setenv(metricresolution.EnabledEnvVar, "false")

	options := requireOptions(t, configmock.NewMock(t), NewDefaultParams(), nil)
	require.Equal(t, aggregator.DefaultFlushInterval, options.FlushInterval)
	require.False(t, options.UseOneSecondDogStatsDAggregation)
}

func TestCreateAgentDemultiplexerOptionsMetricResolutionExperimentEnabled(t *testing.T) {
	t.Setenv(metricresolution.EnabledEnvVar, "true")

	options := requireOptions(t, configmock.NewMock(t), NewDefaultParams(), nil)
	require.Equal(t, time.Second, options.FlushInterval)
	require.True(t, options.UseOneSecondDogStatsDAggregation)
}

func TestCreateAgentDemultiplexerOptionsMetricResolutionExperimentOverridesParams(t *testing.T) {
	t.Setenv(metricresolution.EnabledEnvVar, "true")

	options := requireOptions(t, configmock.NewMock(t), NewDefaultParams(WithFlushInterval(5*time.Second)), nil)
	require.Equal(t, time.Second, options.FlushInterval)
	require.True(t, options.UseOneSecondDogStatsDAggregation)
}

func TestCreateAgentDemultiplexerOptionsMetricResolutionExperimentPreservesZeroFlushInterval(t *testing.T) {
	t.Setenv(metricresolution.EnabledEnvVar, "true")

	options := requireOptions(t, configmock.NewMock(t), NewDefaultParams(WithFlushInterval(0)), nil)
	require.Zero(t, options.FlushInterval)
	require.True(t, options.UseOneSecondDogStatsDAggregation)
}
