// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Node", Name: "nodes", Namespaced: false},
				{Kind: "Namespace", Name: "namespaces", Namespaced: false},
				{Kind: "CustomResource", Name: "customresources/status", Namespaced: false},
				{Kind: "InvalidClusterSubresource", Name: "invalidclusterresource/notvalid", Namespaced: false},
			},
		},
	}

	resourceCache = &ResourceTypeCache{
		kindGroupToType:  make(map[string]string),
		typeGroupToKind:  make(map[string]string),
		clusterResources: make(map[string]ClusterResource),
		discoveryClient:  fakeDiscoveryClient,
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

	assert.Equal(t, ClusterResource{Group: "", APIVersion: "v1", Kind: "Node"}, resourceCache.clusterResources["nodes"])
	assert.Equal(t, ClusterResource{Group: "", APIVersion: "v1", Kind: "Namespace"}, resourceCache.clusterResources["namespaces"])
	assert.Equal(t, ClusterResource{Group: "", APIVersion: "v1", Kind: "CustomResource"}, resourceCache.clusterResources["customresources"])
}

type blockingDiscovery struct {
	*fakediscovery.FakeDiscovery
	gate  chan struct{}
	calls int32 // counts how many times discovery is called
}

func newBlockingDiscovery(fakediscovery *fakediscovery.FakeDiscovery) *blockingDiscovery {
	return &blockingDiscovery{
		FakeDiscovery: fakediscovery,
		gate:          make(chan struct{}),
	}
}

func (b *blockingDiscovery) ServerGroupsAndResources() ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	atomic.AddInt32(&b.calls, 1)
	<-b.gate
	return nil, b.Resources, nil
}

func TestCacheRefreshOnMiss(t *testing.T) {
	resetCache()

	client := fakeclientset.NewClientset()
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)

	// Initial API resources (empty)
	fakeDiscoveryClient.Resources = []*v1.APIResourceList{}
	err := InitializeGlobalResourceTypeCache(fakeDiscoveryClient)
	assert.NoError(t, err, "Initial cache setup should not fail")

	blockingFakeDiscovery := newBlockingDiscovery(fakeDiscoveryClient)
	resourceCache.discoveryClient = blockingFakeDiscovery

	fakeDiscoveryClient.Resources = []*v1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []v1.APIResource{
				{Kind: "Pod", Name: "pods"},
			},
		},
	}

	// fire many goroutines that all miss the cache at the same time
	const goroutineCount = 25
	start := make(chan struct{})
	errCh := make(chan error, goroutineCount)
	var wg sync.WaitGroup
	wg.Add(goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		go func() {
			defer wg.Done()
			<-start // wait until all goroutines are ready
			val, err := resourceCache.getResourceType("Pod", "")
			if err != nil {
				errCh <- fmt.Errorf("got unexpected error: %v", err)
				return
			}
			if val != "pods" {
				errCh <- fmt.Errorf("expected pods, got %q", val)
				return
			}
		}()
	}

	close(start)
	// Give followers time to reach refreshCache() and start waiting on the leader.
	time.Sleep(2 * time.Second)
	close(blockingFakeDiscovery.gate)

	wg.Wait()
	close(errCh)

	got := atomic.LoadInt32(&blockingFakeDiscovery.calls)
	assert.Equal(t, int32(1), got, "expected exactly 1 discovery call")

	for e := range errCh {
		assert.NoError(t, e)
	}

}

func resetCache() {
	cacheOnce = sync.Once{}
	resourceCache = nil
}
