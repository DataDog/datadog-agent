// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
	"k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeDatadogClient struct {
	queryMetricsFunc func(from, to int64, query string) ([]datadog.Series, error)
}

func (d *fakeDatadogClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	if d.queryMetricsFunc != nil {
		return d.queryMetricsFunc(from, to, query)
	}
	return nil, nil
}

func newFakeConfigMapStore(t *testing.T, ns, name string, metrics []custommetrics.ExternalMetricValue) custommetrics.Store {
	store, err := custommetrics.NewConfigMapStore(fake.NewSimpleClientset(), ns, name)
	require.NoError(t, err)
	err = store.SetExternalMetricValues(metrics)
	require.NoError(t, err)
	return store
}

func newMockHPAExternalMetricSource(name, ns, metricName string, labels map[string]string) *v2beta1.HorizontalPodAutoscaler {
	return &v2beta1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v2beta1.HorizontalPodAutoscalerSpec{
			Metrics: []v2beta1.MetricSpec{
				{
					Type: v2beta1.ExternalMetricSourceType,
					External: &v2beta1.ExternalMetricSource{
						MetricName: metricName,
						MetricSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}
}

func TestHPAWatcherRemoveEntryFromStore(t *testing.T) {
	testCases := []struct {
		caseName string
		metrics  []custommetrics.ExternalMetricValue
		hpa      *v2beta1.HorizontalPodAutoscaler
		expected []custommetrics.ExternalMetricValue
	}{
		{
			caseName: "metric exists in store for deleted hpa",
			metrics: []custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{Name: "foo", Namespace: "default"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
			// This HPA references the same metric as the one in the store and has the same name.
			hpa:      newMockHPAExternalMetricSource("foo", "default", "requests_per_s", map[string]string{"bar": "baz"}),
			expected: []custommetrics.ExternalMetricValue{},
		},
		{
			caseName: "metric exists in store for different hpa",
			metrics: []custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{Name: "bar", Namespace: "default"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
			// This HPA references the same metric as the one in the store but has a different name.
			hpa: newMockHPAExternalMetricSource("foo", "default", "requests_per_s", map[string]string{"bar": "baz"}),
			expected: []custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{Name: "bar", Namespace: "default"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			store := newFakeConfigMapStore(t, "default", fmt.Sprintf("test-%d", i), testCase.metrics)
			hpaCl := &HPAWatcherClient{store: store}

			err := hpaCl.removeEntryFromStore([]*v2beta1.HorizontalPodAutoscaler{testCase.hpa})
			require.NoError(t, err)

			allMetrics, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, testCase.expected, allMetrics)
		})
	}
}

func TestHPAWatcherUpdateExternalMetrics(t *testing.T) {
	metricName := "requests_per_s"
	tests := []struct {
		desc     string
		metrics  []custommetrics.ExternalMetricValue
		series   []datadog.Series
		expected []custommetrics.ExternalMetricValue
	}{
		{
			"update invalid metric",
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						{1531492452, 12},
						{1531492486, 14},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Value:      14,
					Valid:      true,
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			store := newFakeConfigMapStore(t, "default", fmt.Sprintf("test-%d", i), tt.metrics)
			datadogClient := &fakeDatadogClient{
				queryMetricsFunc: func(int64, int64, string) ([]datadog.Series, error) {
					return tt.series, nil
				},
			}
			hpaCl := &HPAWatcherClient{datadogClient: datadogClient, store: store}

			hpaCl.updateExternalMetrics()

			externalMetrics, err := store.ListAllExternalMetricValues()

			// Timestamps are always set to time.Now() so we cannot assert the value
			// in a unit test.
			strippedTs := make([]custommetrics.ExternalMetricValue, 0)
			for _, m := range externalMetrics {
				m.Timestamp = 0
				strippedTs = append(strippedTs, m)
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, strippedTs)
		})
	}
}
