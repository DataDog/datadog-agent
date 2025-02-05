// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package resourcetypes

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"

	mockdiscovery "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/resourcetypes/mock"
)

func TestInitializeGlobalResourceTypeCache(t *testing.T) {
	resetCache()
	mockDiscovery := new(mockdiscovery.DiscoveryClient)
	mockDiscovery.On("ServerGroupsAndResources").Return(nil, []*v1.APIResourceList{}, nil)

	tests := []struct {
		name      string
		discovery discovery.DiscoveryInterface
		wantErr   bool
	}{
		{
			name:      "Initialize cache successfully",
			discovery: mockDiscovery,
			wantErr:   false,
		},
		{
			name:      "Re-initialization does nothing",
			discovery: mockDiscovery,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InitializeGlobalResourceTypeCache(tt.discovery)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetResourceType(t *testing.T) {
	resetCache()
	mockDiscovery := new(mockdiscovery.DiscoveryClient)
	mockDiscovery.On("ServerGroupsAndResources").Return(nil, []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
			},
		},
	}, nil)

	err := InitializeGlobalResourceTypeCache(mockDiscovery)
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
			group:   "v1",
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
	mockDiscovery := new(mockdiscovery.DiscoveryClient)
	mockDiscovery.On("ServerGroupsAndResources").Return(nil, []*v1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{Kind: "Deployment", Name: "deployments"},
			},
		},
	}, nil)

	err := InitializeGlobalResourceTypeCache(mockDiscovery)
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
			name:    "Resource not found",
			kind:    "UnknownKind",
			group:   "unknownGroup",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cache.discoverResourceType(tt.kind, tt.group)
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
	mockDiscovery := new(mockdiscovery.DiscoveryClient)
	mockDiscovery.On("ServerGroupsAndResources").Return(nil, []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
				{Kind: "Secret", Name: "secrets"},
				{Kind: "ConfigMap", Name: "configmaps/status"},
			},
		},
	}, nil)

	cache = &ResourceTypeCache{
		kindGroupToType: make(map[string]string),
		discoveryClient: mockDiscovery,
	}

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "Prepopulate cache successfully",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cache.prepopulateCache()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "pods", cache.kindGroupToType["Pod"])
				assert.Equal(t, "secrets", cache.kindGroupToType["Secret"])
				assert.Equal(t, "configmaps", cache.kindGroupToType["ConfigMap"])
			}
		})
	}
}

func TestUtilityFunctions(t *testing.T) {
	resetCache()
	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{
			name: "getAPIGroup with version",
			fn:   func() string { return getAPIGroup("apps/v1") },
			want: "apps",
		},
		{
			name: "getAPIGroup with core API",
			fn:   func() string { return getAPIGroup("v1") },
			want: "",
		},
		{
			name: "getCacheKey with group",
			fn:   func() string { return getCacheKey("Deployment", "apps") },
			want: "Deployment/apps",
		},
		{
			name: "getCacheKey without group",
			fn:   func() string { return getCacheKey("Pod", "") },
			want: "Pod",
		},
		{
			name: "trimSubResource removes subresource",
			fn:   func() string { return trimSubResource("services/status") },
			want: "services",
		},
		{
			name: "trimSubResource with normal resource",
			fn:   func() string { return trimSubResource("pods") },
			want: "pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.fn())
		})
	}
}

func resetCache() {
	cacheOnce = sync.Once{}
	cache = nil
}
