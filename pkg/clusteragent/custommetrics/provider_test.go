// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"fmt"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
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
			"one matching metric with capital letterstored",
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
