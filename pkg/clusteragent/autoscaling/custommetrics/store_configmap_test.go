// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package custommetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
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
	_, err := client.CoreV1().ConfigMaps("default").Create(context.TODO(), cm, metav1.CreateOptions{})
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

func makeConfigMapData(v interface{}) string {
	tests, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(tests)
}

func TestDeprecationStrategy(t *testing.T) {

	client := fake.NewSimpleClientset()
	tests := []struct {
		desc    string
		toStore map[string]string
		output  *MetricsBundle
	}{
		{
			"2 deprecated Metrics",
			map[string]string{
				"external_metric-default-bar-requests_per_s": makeConfigMapData(DeprecatedExternalMetricValue{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					HPA:        ObjectReference{Name: "bar", Namespace: "default"},
				}),
				"external_metric-default-foo-requests_per_s": makeConfigMapData(DeprecatedExternalMetricValue{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					HPA:        ObjectReference{Name: "foo", Namespace: "default"},
				}),
			},
			&MetricsBundle{
				Deprecated: []DeprecatedExternalMetricValue{
					{
						MetricName: "requests_per_s",
						Labels:     map[string]string{"role": "frontend"},
						HPA:        ObjectReference{Name: "foo", Namespace: "default"},
						Valid:      false,
					},
					{
						MetricName: "requests_per_s",
						Labels:     map[string]string{"role": "backend"},
						HPA:        ObjectReference{Name: "bar", Namespace: "default"},
						Valid:      false,
					},
				},
				External: []ExternalMetricValue{},
			},
		},
		{
			"1 deprecated metric 1 external",
			map[string]string{
				"external_metric-default-foo-requests_per_s": makeConfigMapData(DeprecatedExternalMetricValue{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					HPA:        ObjectReference{Name: "foo", Namespace: "default"},
				}),
				"external_metric-watermark-default-bar-requests_per_s": makeConfigMapData(ExternalMetricValue{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "bar", Namespace: "default"},
				}),
			},
			&MetricsBundle{
				Deprecated: []DeprecatedExternalMetricValue{
					{
						MetricName: "requests_per_s",
						Labels:     map[string]string{"role": "backend"},
						HPA:        ObjectReference{Name: "foo", Namespace: "default"},
						Valid:      false,
					},
				},
				External: []ExternalMetricValue{
					{
						MetricName: "requests_per_s",
						Labels:     map[string]string{"role": "frontend"},
						Ref:        ObjectReference{Type: "watermark", Name: "bar", Namespace: "default"},
						Valid:      false,
					},
				},
			},
		},
		{
			"0 deprecated metric 1 external",
			map[string]string{
				"external_metric-watermark-default-foo-requests_per_s": makeConfigMapData(ExternalMetricValue{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				}),
			},
			&MetricsBundle{
				Deprecated: []DeprecatedExternalMetricValue{},
				External: []ExternalMetricValue{
					{
						MetricName: "requests_per_s",
						Labels:     map[string]string{"role": "frontend"},
						Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
						Valid:      false,
					},
				},
			},
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			store, err := NewConfigMapStore(client, "default", fmt.Sprintf("test-%d", i))
			require.NoError(t, err)
			require.NotNil(t, store.(*configMapStore).cm)

			// inject the mocked content
			store.(*configMapStore).cm.Data = tt.toStore
			_, err = client.CoreV1().ConfigMaps("default").Update(context.TODO(), store.(*configMapStore).cm, metav1.UpdateOptions{})
			require.NoError(t, err)

			// Confirm that we are able to isolate the deprecated templates
			m, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.output.External, m.External)
			assert.ElementsMatch(t, tt.output.Deprecated, m.Deprecated)

		})
	}

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
				"external_metric-horizontal-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default"},
				},
				"external_metric-horizontal-default-bar-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Type: "horizontal", Name: "bar", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Type: "horizontal", Name: "bar", Namespace: "default"},
				},
			},
		}, {
			"same metric with different wpas and labels",
			map[string]ExternalMetricValue{
				"external_metric-watermark-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				},
				"external_metric-watermark-default-bar-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Type: "watermark", Name: "bar", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Type: "watermark", Name: "bar", Namespace: "default"},
				},
			},
		},
		{
			"same metric with different owners and same labels",
			map[string]ExternalMetricValue{
				"external_metric-watermark-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				},
				"external_metric-horizontal-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default"},
				},
				"external_metric-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Name: "foo", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default"},
				},
			},
		},
		{
			"different metric with same owner and different labels",
			map[string]ExternalMetricValue{
				"external_metric-watermark-default-foo-requests_per_s_2": {
					MetricName: "requests_per_s_2",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				},
				"external_metric-watermark-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				},
			},
			[]ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"role": "backend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
				}, {
					MetricName: "requests_per_s_2",
					Labels:     map[string]string{"role": "frontend"},
					Ref:        ObjectReference{Type: "watermark", Name: "foo", Namespace: "default"},
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
			assert.ElementsMatch(t, tt.expected, list.External)

			err = store.DeleteExternalMetricValues(list)
			require.NoError(t, err)

			list, err = store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.Empty(t, list.External)
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
					Type:      "horizontal",
					Name:      "bar",
					Namespace: "default",
				},
			},
			output: "external_metric-horizontal-default-bar-foo",
		},
		{
			desc: "custom case",
			emval: ExternalMetricValue{
				MetricName: "FoO",
				Ref: ObjectReference{
					Type:      "horizontal",
					Name:      "bar",
					Namespace: "DefauLt",
				},
			},
			output: "external_metric-horizontal-DefauLt-bar-FoO",
		},
		{
			desc: "default case",
			emval: ExternalMetricValue{
				MetricName: "foo",
				Ref: ObjectReference{
					Type:      "watermark",
					Name:      "bar",
					Namespace: "default",
				},
			},
			output: "external_metric-watermark-default-bar-foo",
		},
		{
			desc: "custom case",
			emval: ExternalMetricValue{
				MetricName: "FoO",
				Ref: ObjectReference{
					Type:      "watermark",
					Name:      "bar",
					Namespace: "DefauLt",
				},
			},
			output: "external_metric-watermark-DefauLt-bar-FoO",
		},
	}

	for i, tt := range test {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			out := ExternalMetricValueKeyFunc(tt.emval)
			assert.Equal(t, tt.output, out)
		})
	}
}
