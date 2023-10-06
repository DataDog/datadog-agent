// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package custommetrics

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

type metricCompare struct {
	name      provider.ExternalMetricInfo
	namespace string
	labels    labels.Set
}

func TestListAllExternalMetrics(t *testing.T) {
	metricName := "m1"
	metric2Name := "m2"
	metric3Name := "m3"
	tests := []struct {
		name      string
		res       []provider.ExternalMetricInfo
		cached    []externalMetric
		timestamp int64
	}{
		{
			name: "no metrics stored",
			res: []provider.ExternalMetricInfo{
				fakeExternalMetric,
			},
			cached: []externalMetric{
				{
					info: fakeExternalMetric,
				},
			},
		},
		{
			name: "one nano metric stored",
			res: []provider.ExternalMetricInfo{
				{
					Metric: metricName,
				},
			},
			cached: []externalMetric{
				{
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
				},
			},
		},
		{
			name: "multiple types",
			cached: []externalMetric{
				{
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
				}, {
					info: provider.ExternalMetricInfo{
						Metric: metric2Name,
					},
				}, {
					info: provider.ExternalMetricInfo{
						Metric: metric3Name,
					},
				},
			},
			res: []provider.ExternalMetricInfo{
				{
					Metric: metricName,
				},
				{
					Metric: metric2Name,
				},
				{
					Metric: metric3Name,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dp := datadogProvider{
				externalMetrics: test.cached,
				isServing:       true,
			}
			output := dp.ListAllExternalMetrics()
			require.Equal(t, len(test.cached), len(output))
		})
	}
}

func TestGetExternalMetric(t *testing.T) {
	metricName := "m1"
	goodLabel := map[string]string{
		"foo": "bar",
	}
	badLabel := map[string]string{
		"oof": "bar",
	}
	ns := "default"
	tests := []struct {
		name          string
		metricsStored []externalMetric
		compared      metricCompare
		expected      []external_metrics.ExternalMetricValue
	}{
		{
			"nothing stored",
			[]externalMetric{},
			metricCompare{},
			[]external_metrics.ExternalMetricValue{},
		},
		{
			"one matching metric stored",
			[]externalMetric{
				{
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: goodLabel,
					},
				},
			},
			metricCompare{
				provider.ExternalMetricInfo{Metric: metricName},
				ns,
				goodLabel,
			},
			[]external_metrics.ExternalMetricValue{
				{
					MetricName:   metricName,
					MetricLabels: goodLabel,
				},
			},
		},
		{
			"one non matching metric stored",
			[]externalMetric{
				{
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: goodLabel,
					},
				},
			},
			metricCompare{
				provider.ExternalMetricInfo{Metric: metricName},
				ns,
				badLabel,
			},
			[]external_metrics.ExternalMetricValue{},
		},
		{
			"one matching name metrics stored",
			[]externalMetric{
				{
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: goodLabel,
					},
				}, {
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: badLabel,
					},
				},
			},
			metricCompare{
				provider.ExternalMetricInfo{Metric: metricName},
				ns,
				goodLabel,
			},
			[]external_metrics.ExternalMetricValue{
				{
					MetricName:   metricName,
					MetricLabels: goodLabel,
				},
			},
		},
		{
			"one non matching labels metrics stored",
			[]externalMetric{
				{
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: goodLabel,
					},
				}, {
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: goodLabel,
					},
				},
			},
			metricCompare{
				provider.ExternalMetricInfo{Metric: metricName},
				ns,
				badLabel,
			},
			[]external_metrics.ExternalMetricValue{},
		},
		{
			"one matching metric with capital letter stored",
			[]externalMetric{
				{
					info: provider.ExternalMetricInfo{
						Metric: "CapitalMetric",
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: goodLabel,
					},
				}, {
					info: provider.ExternalMetricInfo{
						Metric: metricName,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metricName,
						MetricLabels: badLabel,
					},
				},
			},
			metricCompare{
				provider.ExternalMetricInfo{Metric: "capitalmetric"},
				ns,
				goodLabel,
			},
			[]external_metrics.ExternalMetricValue{
				{
					MetricName:   "CapitalMetric",
					MetricLabels: goodLabel,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dp := datadogProvider{
				externalMetrics: test.metricsStored,
				isServing:       true,
				maxAge:          math.MaxInt32, // to avoid flackiness
			}
			output, err := dp.GetExternalMetric(context.Background(), test.compared.namespace, test.compared.labels.AsSelector(), test.compared.name)
			require.NoError(t, err)
			require.Equal(t, len(test.expected), len(output.Items))
			// GetExternalMetric should only return one metric
			if len(output.Items) == 1 {
				require.Equal(t, test.expected[0].MetricName, output.Items[0].MetricName)
				require.True(t, reflect.DeepEqual(test.expected[0].MetricLabels, output.Items[0].MetricLabels))
			}
		})
	}
}
