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
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
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

func newMockHPAExternalMetricSource(name, ns, metricName string, labels map[string]string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
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
		hpa      *autoscalingv2.HorizontalPodAutoscaler
		expected []custommetrics.ExternalMetricValue
	}{
		{
			caseName: "metric exists in store for deleted hpa",
			metrics: []custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPARef:     custommetrics.ObjectReference{Name: "foo", Namespace: "default"},
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
					HPARef:     custommetrics.ObjectReference{Name: "bar", Namespace: "default"},
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
					HPARef:     custommetrics.ObjectReference{Name: "bar", Namespace: "default"},
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

			err := hpaCl.removeEntryFromStore([]*autoscalingv2.HorizontalPodAutoscaler{testCase.hpa})
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

func TestParseHPAs(t *testing.T) {
	test := struct {
		hpa             *autoscalingv2.HorizontalPodAutoscaler
		externalMetrics []custommetrics.ExternalMetricValue
		podsMetrics     []custommetrics.PodsMetricDescriptor
		objectMetrics   []custommetrics.ObjectMetricDescriptor
	}{
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ExternalMetricSourceType,
						External: &autoscalingv2.ExternalMetricSource{
							MetricName: "requests_per_s",
							MetricSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"availability_zone": "us-east-1"},
							},
						},
					},
					{
						Type: autoscalingv2.PodsMetricSourceType,
						Pods: &autoscalingv2.PodsMetricSource{
							MetricName: "conn_opened_per_s",
						},
					},
					{
						Type: autoscalingv2.ObjectMetricSourceType,
						Object: &autoscalingv2.ObjectMetricSource{
							MetricName: "connections",
							Target: autoscalingv2.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "nginx",
							},
						},
					},
				},
			},
		},
		[]custommetrics.ExternalMetricValue{
			{
				MetricName: "requests_per_s",
				Labels:     map[string]string{"availability_zone": "us-east-1"},
				HPARef:     custommetrics.ObjectReference{Name: "foo", Namespace: "default"},
			},
		},
		[]custommetrics.PodsMetricDescriptor{
			{
				MetricName: "conn_opened_per_s",
				HPARef:     custommetrics.ObjectReference{Name: "foo", Namespace: "default"},
			},
		},
		[]custommetrics.ObjectMetricDescriptor{
			{
				MetricName: "connections",
				HPARef:     custommetrics.ObjectReference{Name: "foo", Namespace: "default"},
				DescribedObject: custommetrics.ObjectReference{
					Name:      "nginx",
					Namespace: "default",
				},
			},
		},
	}

	externalMetrics, podsMetrics, objectMetrics := parseHPAs(test.hpa)
	assert.ElementsMatch(t, externalMetrics, test.externalMetrics)
	assert.ElementsMatch(t, podsMetrics, test.podsMetrics)
	assert.ElementsMatch(t, objectMetrics, test.objectMetrics)
}
