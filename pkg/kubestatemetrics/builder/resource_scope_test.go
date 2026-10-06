// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package builder

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

func TestResourceInfoIndexFromDiscovery(t *testing.T) {
	resources := []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true},
				{Name: "pods/status", Kind: "Pod", Namespaced: true},
				{Name: "nodes", Kind: "Node", Namespaced: false},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true},
			},
		},
		{
			GroupVersion: "apps/v1beta1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true},
			},
		},
	}

	resourceInfos := resourceInfoIndexFromDiscovery(resources)
	require.Equal(t, resourceInfo{name: "pods", scope: store.ResourceScopeNamespaced}, resourceInfos[schema.GroupKind{Kind: "Pod"}])
	require.Equal(t, resourceInfo{name: "nodes", scope: store.ResourceScopeCluster}, resourceInfos[schema.GroupKind{Kind: "Node"}])
	require.Equal(t, resourceInfo{name: "deployments", scope: store.ResourceScopeNamespaced}, resourceInfos[schema.GroupKind{Group: "apps", Kind: "Deployment"}])
	require.Len(t, resourceInfos, 3)
}

func TestExpectedTypeToGroupKindForUnstructuredResource(t *testing.T) {
	expectedType := &unstructured.Unstructured{}
	expectedType.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1alpha1",
		Kind:    "Widget",
	})

	groupKind, err := expectedTypeToGroupKind(expectedType)
	require.NoError(t, err)
	require.Equal(t, schema.GroupKind{Group: "example.com", Kind: "Widget"}, groupKind)
}

func TestResourceInfoIndexOmitsAmbiguousGroupKind(t *testing.T) {
	tests := []struct {
		name       string
		v2Resource metav1.APIResource
	}{
		{
			name:       "conflicting scope",
			v2Resource: metav1.APIResource{Name: "widgets", Kind: "Widget", Namespaced: false},
		},
		{
			name:       "conflicting resource name",
			v2Resource: metav1.APIResource{Name: "renamedwidgets", Kind: "Widget", Namespaced: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resources := []*metav1.APIResourceList{
				{
					GroupVersion: "example.com/v1",
					APIResources: []metav1.APIResource{
						{Name: "widgets", Kind: "Widget", Namespaced: true},
					},
				},
				{
					GroupVersion: "example.com/v2",
					APIResources: []metav1.APIResource{test.v2Resource},
				},
			}

			resourceInfos := resourceInfoIndexFromDiscovery(resources)
			_, found := resourceInfos[schema.GroupKind{Group: "example.com", Kind: "Widget"}]
			require.False(t, found)
		})
	}
}
