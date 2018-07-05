// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newMockConfigMap(t *testing.T, name, metricName string, labels map[string]string) *v1.ConfigMap {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
	if metricName == "" || len(labels) == 0 {
		return cm
	}
	em := ExternalMetricValue{
		OwnerRef:   ObjectReference{Name: "foo"},
		MetricName: metricName,
		Labels:     labels,
		Timestamp:  12,
		Value:      1,
		Valid:      false,
	}
	cm.Data = make(map[string]string)
	b, err := json.Marshal(em)
	require.NoError(t, err)
	cm.Data[metricName] = string(b)
	return cm
}

func newMockExternalMetricValue(name string, labels map[string]string) ExternalMetricValue {
	return ExternalMetricValue{
		OwnerRef:   ObjectReference{Name: "foo"},
		MetricName: name,
		Labels:     labels,
		Timestamp:  12,
		Value:      1,
	}
}

func TestNewConfigMapStore(t *testing.T) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}

	client := fake.NewSimpleClientset().CoreV1()
	_, err := client.ConfigMaps("default").Create(cm)
	require.NoError(t, err)

	// configmap already exists
	store, err := NewConfigMapStore(client, "default", "foo")
	require.NoError(t, err)
	require.NotNil(t, store.(*configMapStore).cm)

	// configmap doesn't exist
	store, err = NewConfigMapStore(client, "default", "bar")
	require.NoError(t, err)
	require.NotNil(t, store.(*configMapStore).cm)
}

func TestConfigMapStoreListAllExternalMetrics(t *testing.T) {
	testCases := []struct {
		caseName       string
		configmap      *v1.ConfigMap
		expectedResult []ExternalMetricValue
	}{
		{
			caseName:       "No correct metrics",
			configmap:      newMockConfigMap(t, "foo", "requests_per_sec", nil),
			expectedResult: nil,
		},
		{
			caseName:       "Metric has the expected format",
			configmap:      newMockConfigMap(t, "bar", "requests_per_sec", map[string]string{"bar": "baz"}),
			expectedResult: []ExternalMetricValue{newMockExternalMetricValue("requests_per_sec", map[string]string{"bar": "baz"})},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			client := fake.NewSimpleClientset().CoreV1()

			// create configmap populated with mock data
			cm, err := client.ConfigMaps("default").Create(testCase.configmap)
			require.NoError(t, err)

			store := &configMapStore{
				namespace: "default",
				name:      testCase.configmap.Name,
				client:    client,
				cm:        cm,
			}

			allMetrics, err := store.ListAllExternalMetrics()
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedResult, allMetrics)
		})
	}
}
