// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestInitializeGlobalResourceTypeCache(t *testing.T) {
	resetCache()

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{}

	err := InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err, "First-time cache initialization should not return an error")

	initialCache := resourceCache.kindGroupToType

	fakeDiscoveryClient.Resources = []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{Kind: "Deployment", Name: "deployments"},
			},
		},
	}

	err = InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err, "Re-initialization should not return an error")
	assert.Equal(t, initialCache, resourceCache.kindGroupToType, "Cache should remain unchanged after re-initialization")
}

func TestGetResourceKind(t *testing.T) {
	resetCache()

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
			},
		},

		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{Kind: "Deployment", Name: "deployments"},
			},
		},
	}

	err := InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		resource string
		group    string
		want     string
		wantErr  bool
	}{
		{
			name:     "Cache hit for Pod/v1",
			resource: "pods",
			group:    "",
			want:     "Pod",
			wantErr:  false,
		},
		{
			name:     "Cache hit for Deployment/apps",
			resource: "deployments",
			group:    "apps",
			want:     "Deployment",
			wantErr:  false,
		},
		{
			name:     "Cache miss for unknown kind",
			resource: "UnknownKind",
			group:    "unknownGroup",
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetResourceKind(tt.resource, tt.group)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGetResourceType(t *testing.T) {
	resetCache()

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
			},
		},
	}

	err := InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		kind    string
		group   string
		want    string
		wantErr bool
	}{
		{
			name:    "Cache hit for Pod/v1",
			kind:    "Pod",
			group:   "",
			want:    "pods",
			wantErr: false,
		},
		{
			name:    "Cache miss for unknown kind",
			kind:    "UnknownKind",
			group:   "unknownGroup",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetResourceType(tt.kind, tt.group)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestDiscoverResourceType(t *testing.T) {
	resetCache()

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{Kind: "Deployment", Name: "deployments"},
				{Kind: "StatefulSet", Name: "statefulsets/status"},
				{Kind: "DaemonSet", Name: "daemonsets/proxy"},
			},
		},
	}

	err := InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		kind    string
		group   string
		want    string
		wantErr bool
	}{
		{
			name:    "Find Deployment in apps/v1",
			kind:    "Deployment",
			group:   "apps",
			want:    "deployments",
			wantErr: false,
		},
		{
			name:    "Find StatefulSet with subresource (should trim /status)",
			kind:    "StatefulSet",
			group:   "apps",
			want:    "statefulsets",
			wantErr: false,
		},
		{
			name:    "Invalid subresource (should not be found)",
			kind:    "DaemonSet",
			group:   "apps",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Resource not found",
			kind:    "UnknownKind",
			group:   "unknownGroup",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resourceCache.discoverResourceType(tt.kind, tt.group)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestPrepopulateCache(t *testing.T) {
	resetCache()

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
				{Kind: "Secret", Name: "secrets"},
				{Kind: "ConfigMap", Name: "configmaps/status"},
				{Kind: "InvalidSubresource", Name: "invalidsubresource/notvalid"},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{Kind: "Deployment", Name: "deployments"},
			},
		},
	}

	resourceCache = &ResourceTypeCache{
		kindGroupToType: make(map[string]string),
		typeGroupToKind: make(map[string]string),
		discoveryClient: fakeDiscoveryClient,
	}

	err := resourceCache.prepopulateCache()
	assert.NoError(t, err, "Cache should prepopulate without errors")

	assert.Equal(t, "pods", resourceCache.kindGroupToType["Pod"])
	assert.Equal(t, "secrets", resourceCache.kindGroupToType["Secret"])
	assert.Equal(t, "configmaps", resourceCache.kindGroupToType["ConfigMap"])
	assert.Equal(t, "deployments", resourceCache.kindGroupToType["Deployment/apps"])

	assert.Equal(t, "Pod", resourceCache.typeGroupToKind["pods"])
	assert.Equal(t, "Secret", resourceCache.typeGroupToKind["secrets"])
	assert.Equal(t, "ConfigMap", resourceCache.typeGroupToKind["configmaps"])
	assert.Equal(t, "Deployment", resourceCache.typeGroupToKind["deployments/apps"])
}

func TestCacheRefreshOnMiss(t *testing.T) {
	resetCache()

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)

	// Initial API resources (empty)
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{}

	err := InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err, "Initial cache setup should not fail")

	// Simulate a cache miss
	_, err = resourceCache.getResourceType("Pod", "")
	assert.Error(t, err, "Cache miss should return an error before refresh")

	// Update the discovery client with new API resources
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
			},
		},
	}

	// The next call should trigger a refresh and succeed
	got, err := resourceCache.getResourceType("Pod", "")
	assert.NoError(t, err, "After cache refresh, resource should be found")
	assert.Equal(t, "pods", got, "Returned resource type should match")
}

func resetCache() {
	cacheOnce = sync.Once{}
	resourceCache = nil
}
