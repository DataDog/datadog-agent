// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountNotReadWithoutConfigOption(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               true,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(), nil)

	require.Equal(t, 0, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountFromConfig(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               true,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)

	require.Equal(t, 4, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountDefaultsToOneWhenEnabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline": true,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)

	require.Equal(t, 1, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountDisabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               false,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)

	require.Equal(t, 0, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountFallsBackToOne(t *testing.T) {
	for _, configured := range []int{0, -2} {
		t.Run(fmt.Sprintf("configured_%d", configured), func(t *testing.T) {
			cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
				"dogstatsd_no_aggregation_pipeline":               true,
				"dogstatsd_no_aggregation_pipeline_workers_count": configured,
			})

			options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil)

			require.Equal(t, 1, options.NoAggregationPipelineWorkersCount)
		})
	}
}

func TestCreateAgentDemultiplexerOptionsStoresLookbackFactory(t *testing.T) {
	cfg := configmock.NewMock(t)
	factory := aggregator.DogStatsDLookbackFactory(func(serializer.MetricSerializer) aggregator.DogStatsDLookback {
		return nil
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(), factory)

	require.NotNil(t, options.DogStatsDLookbackFactory)
}
