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

type mockStore struct {
	externalMetrics map[string]custommetrics.ExternalMetricValue
}

func newMockStore(metricName string, labels map[string]string) *mockStore {
	s := &mockStore{}
	em := custommetrics.ExternalMetricValue{
		MetricName:   metricName,
		Labels:       labels,
		HPANamespace: "default",
		HPAName:      "foo",
		Timestamp:    12,
		Value:        1,
		Valid:        false,
	}
	_ = s.SetExternalMetricValues([]custommetrics.ExternalMetricValue{em})
	return s
}

func (s *mockStore) SetExternalMetricValues(added []custommetrics.ExternalMetricValue) error {
	if s.externalMetrics == nil {
		s.externalMetrics = make(map[string]custommetrics.ExternalMetricValue)
	}
	for _, em := range added {
		s.externalMetrics[em.MetricName] = em
	}
	return nil
}

func (s *mockStore) DeleteExternalMetricValues(deleted []custommetrics.ExternalMetricInfo) error {
	for _, info := range deleted {
		delete(s.externalMetrics, info.MetricName)
	}
	return nil
}

func (s *mockStore) ListAllExternalMetricValues() ([]custommetrics.ExternalMetricValue, error) {
	allMetrics := make([]custommetrics.ExternalMetricValue, 0)
	for _, cm := range s.externalMetrics {
		allMetrics = append(allMetrics, cm)
	}
	return allMetrics, nil
}

func (s *mockStore) Update() error { return nil }

func newMockHPAExternalMetricSource(metricName string, labels map[string]string) *v2beta1.HorizontalPodAutoscaler {
	return &v2beta1.HorizontalPodAutoscaler{
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
	hpaCl := HPAWatcherClient{clientSet: fake.NewSimpleClientset()}

	testCases := []struct {
		caseName        string
		store           *mockStore
		hpa             *v2beta1.HorizontalPodAutoscaler
		expectedMetrics map[string]custommetrics.ExternalMetricValue
	}{
		{
			caseName:        "Metric exists, deleting",
			store:           newMockStore("foo", map[string]string{"bar": "baz"}),
			hpa:             newMockHPAExternalMetricSource("foo", map[string]string{"bar": "baz"}),
			expectedMetrics: map[string]custommetrics.ExternalMetricValue{},
		},
		{
			caseName: "Metric is not listed, no-op",
			store:    newMockStore("foobar", map[string]string{"bar": "baz"}),
			hpa:      newMockHPAExternalMetricSource("foo", map[string]string{"bar": "baz"}),
			expectedMetrics: map[string]custommetrics.ExternalMetricValue{
				"foobar": {
					MetricName:   "foobar",
					Labels:       map[string]string{"bar": "baz"},
					HPANamespace: "default",
					HPAName:      "foo",
					Timestamp:    12,
					Value:        1,
					Valid:        false,
				},
			},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			hpaCl.store = testCase.store
			require.NotZero(t, len(testCase.store.externalMetrics))

			err := hpaCl.removeEntryFromStore([]*v2beta1.HorizontalPodAutoscaler{testCase.hpa})
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedMetrics, testCase.store.externalMetrics)
		})
	}
}
