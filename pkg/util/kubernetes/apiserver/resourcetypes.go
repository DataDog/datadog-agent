// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"k8s.io/client-go/discovery"
)

var (
	resourceCache *ResourceTypeCache
	cacheOnce     sync.Once
	initRetry     retry.Retrier
	cacheErr      error
)

// ResourceTypeCache is a global cache to store Kubernetes resource types.
type ResourceTypeCache struct {
	// kindGroupToType maps a resource kind (singular name) and api group to resource type (plural name)
	// this mapping is assumed bijective.
	kindGroupToType map[string]string
	// typeGroupToKind is a reverse map of kindGroupToType
	typeGroupToKind map[string]string
	lock            sync.RWMutex
	discoveryClient discovery.DiscoveryInterface
}

// InitializeGlobalResourceTypeCache initializes the global cache if it hasn't been already.
func InitializeGlobalResourceTypeCache(discoveryClient discovery.DiscoveryInterface) error {
	cacheOnce.Do(func() {
		resourceCache = &ResourceTypeCache{
			kindGroupToType: make(map[string]string),
			typeGroupToKind: make(map[string]string),
			discoveryClient: discoveryClient,
		}

		err := initRetry.SetupRetrier(&retry.Config{
			Name: "ResourceTypeCache_configuration",
			AttemptMethod: func() error {
				err := resourceCache.prepopulateCache()
				if err != nil {
					return fmt.Errorf("failed to prepopulate resource type cache: %w", err)
				}
				return nil
			},
			Strategy:          retry.Backoff,
			InitialRetryDelay: 30 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
		if err != nil {
			cacheErr = fmt.Errorf("failed to initialize resource type cache: %w", err)
		}
	})

	if cacheErr != nil {
		return cacheErr
	}
	err := initRetry.TriggerRetry()
	if err != nil {
		return err.Unwrap()
	}
	return nil
}

// GetResourceType retrieves the resource type for the given kind and group.
func GetResourceType(kind, group string) (string, error) {
	if resourceCache == nil {
		return "", fmt.Errorf("resource type cache is not initialized")
	}
	return resourceCache.getResourceType(kind, group)
}

// GetResourceKind retrieves the kind given the resource plural name and group.
func GetResourceKind(resource, apiGroup string) (string, error) {
	if resourceCache == nil {
		return "", fmt.Errorf("resource type cache is not initialized")
	}

	return resourceCache.getResourceKind(resource, apiGroup)
}

// GetAPIGroup extracts the API group from an API version string (e.g., "apps/v1" → "apps").
// Returns an empty string if no group is present.
func GetAPIGroup(apiVersion string) string {
	var apiGroup string
	if index := strings.Index(apiVersion, "/"); index > 0 {
		apiGroup = apiVersion[:index]
	} else {
		apiGroup = ""
	}
	return apiGroup
}

// getResourceType is the instance method to retrieve a resource type.
func (r *ResourceTypeCache) getResourceType(kind, apiGroup string) (string, error) {
	cacheKey := getCacheKey(kind, apiGroup)

	// Check the cache
	r.lock.RLock()
	resourceType, found := r.kindGroupToType[cacheKey]
	r.lock.RUnlock()
	if found {
		return resourceType, nil
	}

	// Query the API server and update the cache
	resourceType, err := r.discoverResourceType(kind, apiGroup)
	if err != nil {
		return "", err
	}

	r.lock.Lock()
	r.kindGroupToType[cacheKey] = resourceType
	r.lock.Unlock()

	return resourceType, nil
}

// getResourceType is the instance method to retrieve a resource kind.
func (r *ResourceTypeCache) getResourceKind(resource, apiGroup string) (string, error) {
	cacheKey := getCacheKey(resource, apiGroup)

	// Check the cache
	r.lock.RLock()
	kind, found := r.typeGroupToKind[cacheKey]
	r.lock.RUnlock()
	if found {
		return kind, nil
	}

	// Query the API server and update the cache
	resourceKind, err := r.discoverResourceKind(resource, apiGroup)
	if err != nil {
		return "", err
	}

	r.lock.Lock()
	r.typeGroupToKind[cacheKey] = resourceKind
	r.lock.Unlock()

	return resourceKind, nil
}

// discoverResourceKind queries the Kubernetes API server to discover the resource kind based on the plural name
// and the api group.
func (r *ResourceTypeCache) discoverResourceKind(resourceName, group string) (string, error) {
	if r.discoveryClient == nil {
		return "", fmt.Errorf("discovery client is not initialized")
	}
	_, apiResourceLists, err := r.discoveryClient.ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return "", fmt.Errorf("failed to fetch server resources: %w", err)
	}

	for _, list := range apiResourceLists {
		for _, resource := range list.APIResources {
			if !isValidSubresource(resource.Name) {
				continue
			}
			if trimSubResource(resource.Name) == resourceName && GetAPIGroup(list.GroupVersion) == group {
				return resource.Kind, nil
			}
		}
	}
	return "", fmt.Errorf("resource kind not found for resource %s and group %s", resourceName, group)
}

// discoverResourceType queries the Kubernetes API server to discover the resource type.
func (r *ResourceTypeCache) discoverResourceType(kind, group string) (string, error) {
	if r.discoveryClient == nil {
		return "", fmt.Errorf("discovery client is not initialized")
	}
	_, apiResourceLists, err := r.discoveryClient.ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return "", fmt.Errorf("failed to fetch server resources: %w", err)
	}

	for _, list := range apiResourceLists {
		for _, resource := range list.APIResources {
			if !isValidSubresource(resource.Name) {
				continue
			}
			if resource.Kind == kind && GetAPIGroup(list.GroupVersion) == group {
				return trimSubResource(resource.Name), nil
			}
		}
	}
	return "", fmt.Errorf("resource type not found for kind %s and group %s", kind, group)
}

// prepopulateCache pre-fills the cache with all resource types from the discovery client.
func (r *ResourceTypeCache) prepopulateCache() error {
	_, apiResourceLists, err := r.discoveryClient.ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return fmt.Errorf("failed to fetch server resources: %w", err)
	}
	if discovery.IsGroupDiscoveryFailedError(err) {
		log.Debugf("Some groups were skipped during discovery: %v", err)
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	for _, list := range apiResourceLists {
		for _, resource := range list.APIResources {
			if !isValidSubresource(resource.Name) {
				continue
			}
			trimmedResourceType := trimSubResource(resource.Name)
			group := GetAPIGroup(list.GroupVersion)
			kind := resource.Kind

			r.kindGroupToType[getCacheKey(kind, group)] = trimmedResourceType
			r.typeGroupToKind[getCacheKey(trimmedResourceType, group)] = resource.Kind
		}
	}

	return nil
}

func trimSubResource(resourceType string) string {
	if index := strings.Index(resourceType, "/"); index > 0 {
		return resourceType[:index]
	}
	return resourceType
}

// isValidSubresource returns true if there is no subresource or if the subresource is /status
func isValidSubresource(resourceType string) bool {
	if strings.Contains(resourceType, "/") {
		parts := strings.Split(resourceType, "/")
		return len(parts) == 2 && parts[1] == "status"
	}
	return true
}

func getCacheKey(resource, group string) string {
	if group == "" {
		return resource
	}
	return fmt.Sprintf("%s/%s", resource, group)
}
