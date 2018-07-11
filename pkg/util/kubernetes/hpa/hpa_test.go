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
	"k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

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

func TestRemoveEntryFromStore(t *testing.T) {
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
					HPA:        custommetrics.ObjectReference{"foo", "default"},
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
					HPA:        custommetrics.ObjectReference{"bar", "default"},
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
					HPA:        custommetrics.ObjectReference{"bar", "default"},
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
