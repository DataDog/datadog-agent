// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"fmt"
	"strings"
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

func newMockStore(metrics []custommetrics.ExternalMetricValue) *mockStore {
	s := &mockStore{}
	_ = s.SetExternalMetricValues(metrics)
	return s
}

func (s *mockStore) SetExternalMetricValues(added []custommetrics.ExternalMetricValue) error {
	if s.externalMetrics == nil {
		s.externalMetrics = make(map[string]custommetrics.ExternalMetricValue)
	}
	for _, em := range added {
		s.externalMetrics[fmt.Sprintf("%s.%s.%s", em.HPA.Namespace, em.HPA.Name, em.MetricName)] = em
	}
	return nil
}

func (s *mockStore) DeleteExternalMetricValues(deleted []custommetrics.ObjectReference) error {
	for _, obj := range deleted {
		for k := range s.externalMetrics {
			if !strings.HasPrefix(k, fmt.Sprintf("%s.%s", obj.Namespace, obj.Name)) {
				continue
			}
			delete(s.externalMetrics, k)
		}
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
	hpaCl := HPAWatcherClient{clientSet: fake.NewSimpleClientset()}

	testCases := []struct {
		caseName        string
		store           *mockStore
		hpa             *v2beta1.HorizontalPodAutoscaler
		expectedMetrics map[string]custommetrics.ExternalMetricValue
	}{
		{
			caseName: "metric exists in store for deleted hpa",
			store: newMockStore([]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{"foo", "default"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			}),
			// This HPA references the same metric as the one in the store and has the same name.
			hpa:             newMockHPAExternalMetricSource("foo", "default", "requests_per_s", map[string]string{"bar": "baz"}),
			expectedMetrics: map[string]custommetrics.ExternalMetricValue{},
		},
		{
			caseName: "metric exists in store for different hpa",
			store: newMockStore([]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{"bar", "default"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			}),
			// This HPA references the same metric as the one in the store but has a different name.
			hpa: newMockHPAExternalMetricSource("foo", "default", "requests_per_s", map[string]string{"bar": "baz"}),
			expectedMetrics: map[string]custommetrics.ExternalMetricValue{
				"default.bar.requests_per_s": {
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
			hpaCl.store = testCase.store
			require.NotZero(t, len(testCase.store.externalMetrics))

			err := hpaCl.removeEntryFromStore([]*v2beta1.HorizontalPodAutoscaler{testCase.hpa})
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedMetrics, testCase.store.externalMetrics)
		})
	}
}
