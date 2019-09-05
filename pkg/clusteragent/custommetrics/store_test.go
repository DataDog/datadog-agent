// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	assert.NotEmpty(t, store.(*configMapStore).cm.Annotations[storeLastUpdatedAnnotationKey])
}

func TestConfigMapStoreExternalMetrics(t *testing.T) {
	client := fake.NewSimpleClientset()

	tests := []struct {
		desc     string
		metrics  map[string]ExternalMetricValue
		expected []ExternalMetricValue
	}{
		{
			"same metric with different hpas and labels",
			map[string]ExternalMetricValue{
				"external_metric-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				},
				"external_metric-default-bar-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
		},
		{
			"same metric with different owners and same labels",
			map[string]ExternalMetricValue{
				"external_metric-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				},
				"external_metric-default-bar-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "bar", Namespace: "default"},
				},
			},
		},
		{
			"different metric with same owners and different labels",
			map[string]ExternalMetricValue{
				"external_metric-default-foo-requests_per_s_2": {
					MetricName: "requests_per_s_2",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				},
				"external_metric-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				}, {
					MetricName: "requests_per_s_2",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
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

			list, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, list)

			err = store.DeleteExternalMetricValues(list)
			require.NoError(t, err)

			list, err = store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.Empty(t, list)
		})
	}
}

func TestExternalMetricValueKeyFunc(t *testing.T) {
	test := []struct {
		desc   string
		emval  ExternalMetricValue
		output string
	}{
		{
			desc: "default case",
			emval: ExternalMetricValue{
				MetricName: "foo",
				Ref: ObjectReference{
					Name:      "bar",
					Namespace: "default",
				},
			},
			output: "external_metric-default-bar-foo",
		},
		{
			desc: "custom case",
			emval: ExternalMetricValue{
				MetricName: "FoO",
				Ref: ObjectReference{
					Name:      "bar",
					Namespace: "DefauLt",
				},
			},
			output: "external_metric-DefauLt-bar-FoO",
		},
	}

	for i, tt := range test {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			out := ExternalMetricValueKeyFunc(tt.emval)
			assert.Equal(t, tt.output, out)
		})
	}
}
