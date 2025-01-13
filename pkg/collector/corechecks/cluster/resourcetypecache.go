// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

var (
	globalCache     *ResourceTypeCache
	globalCacheOnce sync.Once
	globalCacheErr  error
)

// ResourceTypeCache is a global cache to store Kubernetes resource types.
type ResourceTypeCache struct {
	cache    map[string]string
	lock     sync.RWMutex
	discover discovery.DiscoveryInterface
}

// InitializeGlobalResourceTypeCache initializes the global cache if it hasn't been already.
func InitializeGlobalResourceTypeCache() error {
	globalCacheOnce.Do(func() {
		config, err := rest.InClusterConfig()
		if err != nil {
			globalCacheErr = fmt.Errorf("failed to get in-cluster config: %w", err)
			return
		}
		discover, err := discovery.NewDiscoveryClientForConfig(config)
		if err != nil {
			globalCacheErr = fmt.Errorf("failed to create discovery client: %w", err)
			return
		}

		globalCache = &ResourceTypeCache{
			cache:    make(map[string]string),
			discover: discover,
		}

		// Optionally pre-populate the cache
		err = globalCache.prepopulateCache()
		if err != nil {
			globalCacheErr = fmt.Errorf("failed to prepopulate resource type cache: %w", err)
		}
	})
	log.Infof("Global resource type cache initialized: %v", globalCache)
	return globalCacheErr
}

// GetResourceType retrieves the resource type for the given kind and version.
func GetResourceType(kind, version string) (string, error) {
	if globalCache == nil {
		return "", fmt.Errorf("global resource type cache is not initialized")
	}
	return globalCache.getResourceType(kind, version)
}

// getResourceType is the instance method to retrieve a resource type.
func (r *ResourceTypeCache) getResourceType(kind, version string) (string, error) {
	cacheKey := fmt.Sprintf("%s.%s", kind, version)

	// Check the cache
	r.lock.RLock()
	resourceType, found := r.cache[cacheKey]
	r.lock.RUnlock()
	if found {
		return resourceType, nil
	}

	// Query the API server and update the cache
	resourceType, err := r.discoverResourceType(kind, version)
	if err != nil {
		return "", err
	}

	r.lock.Lock()
	r.cache[cacheKey] = resourceType
	r.lock.Unlock()

	return resourceType, nil
}

// discoverResourceType queries the Kubernetes API server to discover the resource type.
func (r *ResourceTypeCache) discoverResourceType(kind, version string) (string, error) {
	apiResourceLists, err := r.discover.ServerPreferredResources()
	if err != nil {
		return "", fmt.Errorf("failed to fetch server resources: %w", err)
	}

	for _, list := range apiResourceLists {
		if list.GroupVersion == version {
			for _, resource := range list.APIResources {
				if resource.Kind == kind {
					return resource.Name, nil
				}
			}
		}
	}

	return "", fmt.Errorf("resource type not found for kind %s and version %s", kind, version)
}

// prepopulateCache pre-fills the cache with all resource types from the discovery client.
func (r *ResourceTypeCache) prepopulateCache() error {
	_, apiResourceLists, err := r.discover.ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return fmt.Errorf("failed to fetch server resources: %w", err)
	}

	if discovery.IsGroupDiscoveryFailedError(err) {
		log.Warnf("Some groups were skipped during discovery: %v", err)
	}

	// Proceed with populating the cache for valid resource groups
	for _, list := range apiResourceLists {
		for _, resource := range list.APIResources {
			r.cache[fmt.Sprintf("%s.%s", resource.Kind, list.GroupVersion)] = resource.Name
		}
	}

	return nil
}
