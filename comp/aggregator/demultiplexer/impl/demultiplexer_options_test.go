// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountNotReadWithoutConfigOption(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               true,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(), nil, nil, nil)

	require.Equal(t, 0, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountFromConfig(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               true,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil, nil, nil)

	require.Equal(t, 4, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountDefaultsToOneWhenEnabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline": true,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil, nil, nil)

	require.Equal(t, 1, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountDisabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"dogstatsd_no_aggregation_pipeline":               false,
		"dogstatsd_no_aggregation_pipeline_workers_count": 4,
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil, nil, nil)

	require.Equal(t, 0, options.NoAggregationPipelineWorkersCount)
}

func TestCreateAgentDemultiplexerOptionsNoAggWorkerCountFallsBackToOne(t *testing.T) {
	for _, configured := range []int{0, -2} {
		t.Run(fmt.Sprintf("configured_%d", configured), func(t *testing.T) {
			cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
				"dogstatsd_no_aggregation_pipeline":               true,
				"dogstatsd_no_aggregation_pipeline_workers_count": configured,
			})

			options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(WithDogstatsdNoAggregationPipelineConfig()), nil, nil, nil)

			require.Equal(t, 1, options.NoAggregationPipelineWorkersCount)
		})
	}
}

func TestCreateAgentDemultiplexerOptionsStoresLookbackFactory(t *testing.T) {
	cfg := configmock.NewMock(t)
	factory := aggregator.DogStatsDLookbackFactory(func(serializer.MetricSerializer) aggregator.DogStatsDLookback {
		return nil
	})

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(), factory, nil, nil)

	require.NotNil(t, options.DogStatsDLookbackFactory)
}

type recordingFinalDogStatsDSerieObserver struct{}

func (*recordingFinalDogStatsDSerieObserver) ObserveFinalDogStatsDSerie(*metrics.Serie) {}

type noopClientDropDetector struct{}

func (*noopClientDropDetector) CompleteFinalDogStatsDSerieFlush() {}
func (*noopClientDropDetector) ObserveClientBytes(string, dogstatsdclientdropdetector.ClientByteMetric, float64) {
}

func TestCreateAgentDemultiplexerOptionsStoresFinalDogStatsDSerieObservers(t *testing.T) {
	cfg := configmock.NewMock(t)
	observers := []aggregator.FinalDogStatsDSerieObserver{&recordingFinalDogStatsDSerieObserver{}}

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(), nil, observers, nil)

	require.Equal(t, observers, options.FinalDogStatsDSerieObservers)
}

func TestCreateAgentDemultiplexerOptionsStoresClientDropDetector(t *testing.T) {
	cfg := configmock.NewMock(t)
	detector := &noopClientDropDetector{}

	options := createAgentDemultiplexerOptions(cfg, NewDefaultParams(), nil, nil, detector)

	require.Equal(t, detector, options.FinalDogStatsDSerieFlushListener)
}
