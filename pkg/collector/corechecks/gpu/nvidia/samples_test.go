// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

func requireMetrics(t *testing.T, samples []Sample) []*Metric {
	t.Helper()

	metrics := make([]*Metric, 0, len(samples))
	for _, sample := range samples {
		metric, ok := sample.(*Metric)
		require.Truef(t, ok, "expected metric sample, got %T", sample)
		metrics = append(metrics, metric)
	}
	return metrics
}

func TestMetricSampleOwnsMetadata(t *testing.T) {
	tags := []string{"source:nvml"}
	workloads := []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "1"}}
	metric := NewMetric("utilization", 1, ddmetrics.GaugeType, Low, tags, workloads)

	tags[0] = "source:changed"
	workloads[0].ID = "2"
	require.Equal(t, []string{"source:nvml"}, metric.tags)
	require.Equal(t, "1", metric.AssociatedWorkloads()[0].ID)

	returnedWorkloads := metric.AssociatedWorkloads()
	returnedWorkloads[0].ID = "3"
	require.Equal(t, "1", metric.AssociatedWorkloads()[0].ID)

	clone, ok := metric.Clone().(*Metric)
	require.True(t, ok)
	clone.AppendTags([]string{"scope:clone"})
	clone.associatedWorkloads[0].ID = "4"

	require.Equal(t, []string{"source:nvml"}, metric.tags)
	require.Equal(t, "1", metric.AssociatedWorkloads()[0].ID)
}

func TestRemoveDuplicateSamplesKeepsIndependentSampleTypes(t *testing.T) {
	samples := map[CollectorName][]Sample{
		"low": {
			NewMetric("errors", 1, ddmetrics.CountType, Low, nil, nil),
			NewHistogramSample("errors", 2, [2]float64{0, 1}, true, false, Low, nil, nil),
			NewHistogramSample("errors", 3, [2]float64{1, 2}, true, false, Low, nil, nil),
		},
		"high": {
			NewMetric("errors", 4, ddmetrics.CountType, High, nil, nil),
		},
	}

	result := RemoveDuplicateSamples(samples)
	require.Len(t, result, 3)

	var metricCount, histogramCount int
	for _, sample := range result {
		switch sample := sample.(type) {
		case *Metric:
			metricCount++
			require.Equal(t, 4.0, sample.Value)
		case *HistogramSample:
			histogramCount++
		default:
			require.Failf(t, "unexpected sample type", "%T", sample)
		}
	}
	require.Equal(t, 1, metricCount)
	require.Equal(t, 2, histogramCount)
}
