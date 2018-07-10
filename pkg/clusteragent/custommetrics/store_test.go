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

func TestConfigMapStoreExternalMetrics(t *testing.T) {
	client := fake.NewSimpleClientset().CoreV1()

	tests := []struct {
		desc     string
		metrics  []ExternalMetricValue
		expected []ExternalMetricValue
	}{
		{
			"same metric with different owners and labels",
			[]ExternalMetricValue{
				{
					OwnerRef:   ObjectReference{Name: "foo", UID: "d7c6d419-7ee8-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "frontend"},
				},
				{
					OwnerRef:   ObjectReference{Name: "bar", UID: "39f7b47d-7eeb-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "backend"},
				},
			},
			[]ExternalMetricValue{
				{
					OwnerRef:   ObjectReference{Name: "foo", UID: "d7c6d419-7ee8-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "frontend"},
				},
				{
					OwnerRef:   ObjectReference{Name: "bar", UID: "39f7b47d-7eeb-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "backend"},
				},
			},
		},
		{
			"same metric with different owners and same labels",
			[]ExternalMetricValue{
				{
					OwnerRef:   ObjectReference{Name: "foo", UID: "d7c6d419-7ee8-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "frontend"},
				},
				{
					OwnerRef:   ObjectReference{Name: "bar", UID: "39f7b47d-7eeb-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "frontend"},
				},
			},
			[]ExternalMetricValue{
				{
					OwnerRef:   ObjectReference{Name: "foo", UID: "d7c6d419-7ee8-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "frontend"},
				},
				{
					OwnerRef:   ObjectReference{Name: "bar", UID: "39f7b47d-7eeb-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "frontend"},
				},
			},
		},
		{
			"same metric with same owners and different labels",
			[]ExternalMetricValue{
				{
					OwnerRef:   ObjectReference{Name: "foo", UID: "d7c6d419-7ee8-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "frontend"},
				},
				{
					OwnerRef:   ObjectReference{Name: "foo", UID: "d7c6d419-7ee8-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "backend"},
				},
			},
			[]ExternalMetricValue{
				{
					OwnerRef:   ObjectReference{Name: "foo", UID: "d7c6d419-7ee8-11e8-a56b-42010a800227"},
					MetricName: "requests_per_sec",
					Labels:     map[string]string{"role": "backend"},
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			store, err := NewConfigMapStore(client, "default", fmt.Sprintf("test-%d", i))
			require.NoError(t, err)
			require.NotNil(t, store.(*configMapStore).cm)

			err = store.Begin(func(tx Tx) {
				for _, em := range tt.metrics {
					tx.Set(em)
				}
			})
			require.NoError(t, err)

			allMetrics, err := store.ListAllExternalMetrics()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, allMetrics)

			err = store.Begin(func(tx Tx) {
				for _, em := range tt.metrics {
					tx.Delete(string(em.OwnerRef.UID), em.MetricName)
				}
			})
			require.NoError(t, err)
			assert.Zero(t, len(store.(*configMapStore).cm.Data))
		})
	}
}
