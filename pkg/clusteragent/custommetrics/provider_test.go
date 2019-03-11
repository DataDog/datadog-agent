// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type mockDatadogProvider struct {
	mockListAllExternalMetricValues func() ([]ExternalMetricValue, error)
	metrics                         []ExternalMetricValue
}

func (m *mockDatadogProvider) ListAllExternalMetricValues() ([]ExternalMetricValue, error) {
	return m.mockListAllExternalMetricValues()
}
func (m *mockDatadogProvider) SetExternalMetricValues([]ExternalMetricValue) error    { return nil }
func (m *mockDatadogProvider) DeleteExternalMetricValues([]ExternalMetricValue) error { return nil }
func (m *mockDatadogProvider) GetMetrics() (*MetricsBundle, error)                    { return nil, nil }

type metricCompare struct {
	name      provider.ExternalMetricInfo
	namespace string
	labels    labels.Set
}

func TestListAllExternalMetrics(t *testing.T) {
	metricName := "m1"
	metric2Name := "m2"
	metric3Name := "m3"
	labels := map[string]string{
		"foo": "bar",
	}
	cachedQuantity, _ := resource.ParseQuantity("0.00000000003")
	tests := []struct {
		name          string
		metricsStored []ExternalMetricValue
		cached        []externalMetric
		expected      []externalMetric
		timestamp     int64
	}{
		{
			name:          "no metrics stored",
			metricsStored: []ExternalMetricValue{},
			cached:        []externalMetric{},
			expected:      nil,
		},
		{
			name: "last call is too recent",
			metricsStored: []ExternalMetricValue{
				{
					MetricName: metricName,
					Labels:     labels,
					Valid:      true,
				}},
			cached: []externalMetric{{
				info: provider.ExternalMetricInfo{
					Metric: metricName,
				},
				value: external_metrics.ExternalMetricValue{
					MetricName:   metricName,
					MetricLabels: labels,
					Value:        cachedQuantity,
				},
			}},
			timestamp: metav1.Now().Unix(),
			expected: []externalMetric{{
				info: provider.ExternalMetricInfo{
					Metric: metricName,
				},
				value: external_metrics.ExternalMetricValue{
					MetricName:   metricName,
					MetricLabels: labels,
					Value:        cachedQuantity,
				},
			}},
		},
		{
			name: "one nano metric stored",
			metricsStored: []ExternalMetricValue{
				{
					MetricName: metricName,
					Value:      float64(0.000000001),
					Labels:     labels,
					Valid:      true,
				}},
			cached: []externalMetric{{}},
			expected: []externalMetric{{
				info: provider.ExternalMetricInfo{
					Metric: metricName,
				},
				value: external_metrics.ExternalMetricValue{
					MetricName:   metricName,
					MetricLabels: labels,
					Value:        resource.MustParse("1e-09"),
				},
			},
			},
			timestamp: 0,
		},
		{
			name: "multiple types",
			metricsStored: []ExternalMetricValue{
				{
					MetricName: metricName,
					Value:      float64(0.012),
					Labels:     labels,
					Valid:      true,
				}, {
					MetricName: metric2Name,
					Value:      float64(1),
					Labels:     labels,
					Valid:      true,
				}, {
					MetricName: metric3Name,
					Value:      float64(1000001),
					Labels:     labels,
					Valid:      true,
				}, {
					MetricName: "unexpected",
					Labels:     labels,
					Valid:      false,
				},
			},
			expected: []externalMetric{{
				info: provider.ExternalMetricInfo{
					Metric: metricName,
				},
				value: external_metrics.ExternalMetricValue{
					MetricName:   metricName,
					MetricLabels: labels,
					Value:        *resource.NewMilliQuantity(12, resource.DecimalSI),
				},
			}, {
				info: provider.ExternalMetricInfo{
					Metric: metric2Name,
				},
				value: external_metrics.ExternalMetricValue{
					MetricName:   metric2Name,
					MetricLabels: labels,
					Value:        resource.MustParse("1"),
				},
			},
				{
					info: provider.ExternalMetricInfo{
						Metric: metric3Name,
					},
					value: external_metrics.ExternalMetricValue{
						MetricName:   metric3Name,
						MetricLabels: labels,
						Value:        resource.MustParse("1.000001e+06"),
					}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &mockDatadogProvider{
				mockListAllExternalMetricValues: func() ([]ExternalMetricValue, error) {
					return test.metricsStored, nil
				},
			}
			dp := datadogProvider{
				store:           Store(m),
				timestamp:       test.timestamp,
				maxAge:          int64(300),
				externalMetrics: test.cached,
			}
			output := dp.ListAllExternalMetrics()
			require.Equal(t, len(test.expected), len(output))

			for _, q := range dp.externalMetrics {
				for _, val := range test.expected {
					if val.info.Metric == q.info.Metric {
						require.Equal(t, val.value, q.value)
					}
				}
			}
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
		metricsStored []ExternalMetricValue
		compared      metricCompare
		expected      []external_metrics.ExternalMetricValue
	}{
		{
			"nothing stored",
			nil,
			metricCompare{},
			[]external_metrics.ExternalMetricValue{},
		},
		{
			"one matching metric stored",
			[]ExternalMetricValue{
				{
					MetricName: metricName,
					Labels:     goodLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}},
			metricCompare{
				provider.ExternalMetricInfo{Metric: metricName},
				ns,
				goodLabel,
			},
			[]external_metrics.ExternalMetricValue{
				{
					MetricName:   metricName,
					MetricLabels: goodLabel,
				}},
		},
		{
			"one non matching metric stored",
			[]ExternalMetricValue{
				{
					MetricName: metricName,
					Labels:     goodLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}},
			metricCompare{
				provider.ExternalMetricInfo{Metric: metricName},
				ns,
				badLabel,
			},
			[]external_metrics.ExternalMetricValue{},
		},
		{
			"one matching name metrics stored",
			[]ExternalMetricValue{
				{
					MetricName: metricName,
					Labels:     goodLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}, {
					MetricName: metricName,
					Labels:     badLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}},
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
			[]ExternalMetricValue{
				{
					MetricName: metricName,
					Labels:     goodLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}, {
					MetricName: metricName,
					Labels:     goodLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}},
			metricCompare{
				provider.ExternalMetricInfo{Metric: metricName},
				ns,
				badLabel,
			},
			[]external_metrics.ExternalMetricValue{},
		},
		{
			"one matching metric with capital letter stored",
			[]ExternalMetricValue{
				{
					MetricName: "CapitalMetric",
					Labels:     goodLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}, {
					MetricName: metricName,
					Labels:     badLabel,
					HPA:        ObjectReference{Namespace: ns},
					Valid:      true,
				}},
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
			m := &mockDatadogProvider{
				mockListAllExternalMetricValues: func() ([]ExternalMetricValue, error) {
					return test.metricsStored, nil
				},
			}
			dp := datadogProvider{
				store: Store(m),
			}
			output, err := dp.GetExternalMetric(test.compared.namespace, test.compared.labels.AsSelector(), test.compared.name)
			require.NoError(t, err)
			require.Equal(t, len(test.expected), len(output.Items))
			// GetExternalMetric should only return one metric
			if len(output.Items) == 1 {
				require.Equal(t, test.expected[0].MetricName, output.Items[0].MetricName)
				fmt.Printf("test is %#v \n \n output is %#v \n \n", test.expected[0].MetricLabels, output.Items[0].MetricLabels)
				require.True(t, reflect.DeepEqual(test.expected[0].MetricLabels, output.Items[0].MetricLabels))
			}
		})
	}
}
