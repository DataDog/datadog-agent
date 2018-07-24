// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewConfigMapStore(t *testing.T) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}

	client := fake.NewSimpleClientset()
	_, err := client.CoreV1().ConfigMaps("default").Create(cm)
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

func TestConfigMapStoreExternalMetrics(t *testing.T) {
	client := fake.NewSimpleClientset()

	tests := []struct {
		desc     string
		metrics  []ExternalMetricValue
		expected []ExternalMetricValue
	}{
		{
			"same metric with different hpas and labels",
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPARef:     ObjectReference{Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					HPARef:     ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPARef:     ObjectReference{Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					HPARef:     ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
		},
		{
			"same metric with different owners and same labels",
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPARef:     ObjectReference{Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPARef:     ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPARef:     ObjectReference{Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPARef:     ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
		},
		{
			"same metric with same owners and different labels",
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPARef:     ObjectReference{Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					HPARef:     ObjectReference{Name: "foo", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					HPARef:     ObjectReference{Name: "foo", Namespace: "default"},
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			store, err := NewConfigMapStore(client, "default", fmt.Sprintf("test-%d", i))
			require.NoError(t, err)
			require.NotNil(t, store.(*configMapStore).cm)

			err = store.SetExternalMetricValues(tt.metrics)
			require.NoError(t, err)

			allMetrics, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, allMetrics)
		})
	}
}

func TestConfigMapStorePurge(t *testing.T) {
	client := fake.NewSimpleClientset()

	store, err := NewConfigMapStore(client, "default", "bag-o-things")
	require.NoError(t, err)

	fooRef := ObjectReference{
		Namespace: "foo",
		Name:      "default",
	}

	barRef := ObjectReference{
		Namespace: "bar",
		Name:      "default",
	}

	externalMetrics := []ExternalMetricValue{
		{
			MetricName: "requests_per_s",
			Labels:     map[string]string{"availability_zone": "us-east-1"},
			HPARef:     fooRef,
		},
	}
	err = store.SetExternalMetricValues(externalMetrics)
	require.NoError(t, err)

	podsDescs := []PodsMetricDescriptor{
		{
			MetricName: "conn_opened_per_s",
			HPARef:     barRef,
		},
	}

	objectDescs := []ObjectMetricDescriptor{
		{
			MetricName:      "conn_opened_per_s",
			DescribedObject: ObjectReference{Name: "nginx", Namespace: "default"},
			HPARef:          barRef,
		},
	}
	err = store.SetMetricDescriptors(podsDescs, objectDescs)
	require.NoError(t, err)

	// Purging both references should delete all data in configmap.
	err = store.Purge([]ObjectReference{fooRef, barRef})
	require.NoError(t, err)
	assert.Zero(t, len(store.(*configMapStore).cm.Data))
}
