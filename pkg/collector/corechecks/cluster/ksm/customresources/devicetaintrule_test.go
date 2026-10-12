// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestDeviceTaintRuleAPIVersion verifies the version detection: 1.37+ clusters
// serve the resource type, older clusters do not.
func TestDeviceTaintRuleAPIVersion(t *testing.T) {
	tests := []struct {
		name      string
		resources []*metav1.APIResourceList
		expected  string
	}{
		{
			name: "1.37+ with devicetaintrules",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "resource.k8s.io/v1",
					APIResources: []metav1.APIResource{
						{Name: "resourceclaims"},
						{Name: "resourceslices"},
						{Name: "devicetaintrules"},
					},
				},
			},
			expected: "v1",
		},
		{
			name: "1.36 without devicetaintrules",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "resource.k8s.io/v1",
					APIResources: []metav1.APIResource{
						{Name: "resourceclaims"},
						{Name: "resourceslices"},
					},
				},
			},
			expected: "",
		},
		{
			name: "no DRA group at all",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{Name: "deployments"},
					},
				},
			},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DeviceTaintRuleAPIVersion(tt.resources))
		})
	}
}

// TestDeviceTaintRuleInfoMetric verifies the info metric extracts the taint
// rule fields (driver, key, effect) from a DeviceTaintRule object.
func TestDeviceTaintRuleInfoMetric(t *testing.T) {
	f := &deviceTaintRuleFactory{apiVersion: "v1"}

	rule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "resource.k8s.io/v1",
			"kind":       "DeviceTaintRule",
			"metadata": map[string]interface{}{
				"name": "unhealthy-gpu",
			},
			"spec": map[string]interface{}{
				"deviceSelector": map[string]interface{}{
					"driver": "gpu.nvidia.com",
				},
				"taint": map[string]interface{}{
					"key":    "gpu.nvidia.com/unhealthy",
					"value":  "Broken",
					"effect": "NoExecute",
				},
			},
		},
	}

	// Call the production generator, not a reimplementation: the test must
	// fail if the generator's field paths, labels, or value change.
	family := generatorByName(t, f.MetricFamilyGenerators(), "kube_devicetaintrule_info").Generate(rule)

	require.Len(t, family.Metrics, 1)
	m := family.Metrics[0]
	assert.Equal(t, float64(1), m.Value)
	assert.Equal(t, []string{"unhealthy-gpu", "gpu.nvidia.com", "gpu.nvidia.com/unhealthy", "NoExecute"}, m.LabelValues)
}
