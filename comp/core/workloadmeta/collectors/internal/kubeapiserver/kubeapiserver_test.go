// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestStoreGenerators(t *testing.T) {
	// Define tests
	tests := []struct {
		name                    string
		cfg                     map[string]bool
		expectedStoresGenerator []storeGenerator
	}{
		{
			name: "All configurations disabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": false,
				"language_detection.reporting.enabled":  false,
				"language_detection.enabled":            false,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore},
		},
		{
			name: "All configurations disabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": false,
				"language_detection.reporting.enabled":  false,
				"language_detection.enabled":            true,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore},
		},
		{
			name: "Kubernetes tags enabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": true,
				"language_detection.reporting.enabled":  false,
				"language_detection.enabled":            true,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore, newPodStore},
		},
		{
			name: "Language detection enabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": false,
				"language_detection.reporting.enabled":  true,
				"language_detection.enabled":            true,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore, newDeploymentStore},
		},
		{
			name: "Language detection enabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": false,
				"language_detection.reporting.enabled":  true,
				"language_detection.enabled":            false,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore},
		},
		{
			name: "Kube namespace collection enabled",
			cfg: map[string]bool{
				"kubernetes_namespace_collection_enabled": true,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore, newNamespaceStore},
		},
		{
			name: "All configurations enabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": true,
				"language_detection.reporting.enabled":  true,
				"language_detection.enabled":            true,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore, newPodStore, newDeploymentStore},
		},
	}

	// Run test for each testcase
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
			for k, v := range tt.cfg {
				cfg.SetWithoutSource(k, v)
			}
			expectedStores := collectResultStoreGenerator(tt.expectedStoresGenerator)
			stores := collectResultStoreGenerator(storeGenerators(cfg))

			assert.Equal(t, expectedStores, stores)
		})
	}
}

func collectResultStoreGenerator(funcs []storeGenerator) []*reflectorStore {
	var stores []*reflectorStore
	for _, f := range funcs {
		_, s := f(nil, nil, nil)
		stores = append(stores, s)
	}
	return stores
}

func Test_metadataCollectionGVRs_WithFunctionalDiscovery(t *testing.T) {
	tests := []struct {
		name                  string
		apiServerResourceList []*metav1.APIResourceList
		expectedGVRs          []schema.GroupVersionResource
		cfg                   map[string]interface{}
	}{
		{
			name:                  "no requested resources, no resources at all!",
			apiServerResourceList: []*metav1.APIResourceList{},
			expectedGVRs:          []schema.GroupVersionResource{},
			cfg: map[string]interface{}{
				"cluster_agent.kube_metadata_collection.enabled":   true,
				"cluster_agent.kube_metadata_collection.resources": "",
			},
		},
		{
			name:                  "requested resources, but no resources at all!",
			apiServerResourceList: []*metav1.APIResourceList{},
			expectedGVRs:          []schema.GroupVersionResource{},
			cfg: map[string]interface{}{
				"cluster_agent.kube_metadata_collection.enabled":   true,
				"cluster_agent.kube_metadata_collection.resources": "deployments",
			},
		},
		{
			name: "only one resource (deployments), only one version, correct resource requested",
			apiServerResourceList: []*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
			},
			expectedGVRs: []schema.GroupVersionResource{{Resource: "deployments", Group: "apps", Version: "v1"}},
			cfg: map[string]interface{}{
				"cluster_agent.kube_metadata_collection.enabled":   true,
				"cluster_agent.kube_metadata_collection.resources": "deployments",
			},
		},
		{
			name: "only one resource (deployments), only one version, wrong resource requested",
			apiServerResourceList: []*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
			},
			expectedGVRs: []schema.GroupVersionResource{},
			cfg: map[string]interface{}{
				"cluster_agent.kube_metadata_collection.enabled":   true,
				"cluster_agent.kube_metadata_collection.resources": "daemonsets",
			},
		},
		{
			name: "multiple resources (deployments, statefulsets), multiple versions, all resources requested",
			apiServerResourceList: []*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1beta1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "statefulsets",
							Kind:       "StatefulSet",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1beta1",
					APIResources: []metav1.APIResource{
						{
							Name:       "statefulsets",
							Kind:       "StatefulSet",
							Namespaced: true,
						},
					},
				},
			},
			expectedGVRs: []schema.GroupVersionResource{
				{Resource: "deployments", Group: "apps", Version: "v1"},
				{Resource: "statefulsets", Group: "apps", Version: "v1"},
			},
			cfg: map[string]interface{}{
				"cluster_agent.kube_metadata_collection.enabled":   true,
				"cluster_agent.kube_metadata_collection.resources": "deployments statefulsets",
			},
		},
		{
			name: "multiple resources (deployments, statefulsets), multiple versions, only one resource requested",
			apiServerResourceList: []*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1beta1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "statefulsets",
							Kind:       "StatefulSet",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1beta1",
					APIResources: []metav1.APIResource{
						{
							Name:       "statefulsets",
							Kind:       "StatefulSet",
							Namespaced: true,
						},
					},
				},
			},
			expectedGVRs: []schema.GroupVersionResource{{Resource: "deployments", Group: "apps", Version: "v1"}},
			cfg: map[string]interface{}{
				"cluster_agent.kube_metadata_collection.enabled":   true,
				"cluster_agent.kube_metadata_collection.resources": "deployments",
			},
		},
		{
			name: "multiple resources (deployments, statefulsets), multiple versions, two resources requested (one with a typo)",
			apiServerResourceList: []*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1beta1",
					APIResources: []metav1.APIResource{
						{
							Name:       "deployments",
							Kind:       "Deployment",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "statefulsets",
							Kind:       "StatefulSet",
							Namespaced: true,
						},
					},
				},
				{
					GroupVersion: "apps/v1beta1",
					APIResources: []metav1.APIResource{
						{
							Name:       "statefulsets",
							Kind:       "StatefulSet",
							Namespaced: true,
						},
					},
				},
			},
			expectedGVRs: []schema.GroupVersionResource{
				{Resource: "deployments", Group: "apps", Version: "v1"},
			},
			cfg: map[string]interface{}{
				"cluster_agent.kube_metadata_collection.enabled":   true,
				"cluster_agent.kube_metadata_collection.resources": "deployments statefulsetsy",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
			for k, v := range test.cfg {
				cfg.SetWithoutSource(k, v)
			}

			client := fakeclientset.NewSimpleClientset()
			fakeDiscoveryClient, ok := client.Discovery().(*fakediscovery.FakeDiscovery)
			assert.Truef(t, ok, "Failed to initialise fake discovery client")

			fakeDiscoveryClient.Resources = test.apiServerResourceList

			discoveredGVRs, err := metadataCollectionGVRs(cfg, fakeDiscoveryClient)
			assert.NoErrorf(t, err, "Function should not have returned an error")

			assert.Truef(t, reflect.DeepEqual(discoveredGVRs, test.expectedGVRs), "Expected %v but got %v.", test.expectedGVRs, discoveredGVRs)
		})
	}
}

/*
type MockFailingDiscoveryClient struct {
	fakediscovery.FakeDiscovery
}

func (m *MockFailingDiscoveryClient) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	return nil, nil, fmt.Errorf("timeout error")
}

func Test_metadataCollectionGVRs_WithDiscoveryFailure(t *testing.T) {
	discoveryClient := &MockFailingDiscoveryClient{}
	discoveredGVRs, err := discoverGVRs(discoveryClient, []string{})
	assert.Errorf(t, err, "Function should have returned an error")
	assert.Nilf(t, discoveredGVRs, "Discovered GVRs should be nil")
}
*/
