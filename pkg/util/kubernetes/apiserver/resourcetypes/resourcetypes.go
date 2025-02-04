// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package resourcetypes provides utilities for resolving Kubernetes resourceTypes using the discovery client.
package resourcetypes

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/discovery"
)

var (
	cache    *ResourceTypeCache
	cacheErr error
)

// ResourceTypeCache is a global cache to store Kubernetes resource types.
type ResourceTypeCache struct {
	kindGroupToType map[string]string
	lock            sync.RWMutex
	discoveryClient discovery.DiscoveryInterface
}

// InitializeGlobalResourceTypeCache initializes the global cache if it hasn't been already.
func InitializeGlobalResourceTypeCache(discoveryClient discovery.DiscoveryInterface) error {
	cache = &ResourceTypeCache{
		kindGroupToType: make(map[string]string),
		discoveryClient: discoveryClient,
	}

	// Optionally pre-populate the cache
	err := cache.prepopulateCache()
	if err != nil {
		cacheErr = fmt.Errorf("failed to prepopulate resource type cache: %w", err)
	}
	return cacheErr
}

// GetResourceType retrieves the resource type for the given kind and group.
func GetResourceType(kind, apiVersion string) (string, error) {
	if cache == nil {
		return "", fmt.Errorf("resource type cache is not initialized")
	}
	return cache.getResourceType(kind, apiVersion)
}

// getResourceType is the instance method to retrieve a resource type.
func (r *ResourceTypeCache) getResourceType(kind, apiVersion string) (string, error) {
	group := getAPIGroup(apiVersion)
	cacheKey := getCacheKey(kind, group)

	// Check the cache
	r.lock.RLock()
	resourceType, found := r.kindGroupToType[cacheKey]
	r.lock.RUnlock()
	if found {
		return resourceType, nil
	}

	// Query the API server and update the cache
	resourceType, err := r.discoverResourceType(kind, group)
	if err != nil {
		return "", err
	}

	r.lock.Lock()
	r.kindGroupToType[cacheKey] = resourceType
	r.lock.Unlock()

	return resourceType, nil
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
			if resource.Kind == kind && getAPIGroup(list.GroupVersion) == group {
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
		log.Warnf("Some groups were skipped during discovery: %v", err)
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	// Proceed with populating the cache for valid resource groups
	for _, list := range apiResourceLists {
		for _, resource := range list.APIResources {
			if !isValidSubresource(resource.Name) {
				continue
			}
			trimmedResourceType := trimSubResource(resource.Name)
			cacheKey := getCacheKey(resource.Kind, getAPIGroup(list.GroupVersion))
			r.kindGroupToType[cacheKey] = trimmedResourceType
		}
	}

	return nil
}

func trimSubResource(resourceType string) string {
	if strings.Contains(resourceType, "/") {
		return strings.Split(resourceType, "/")[0]
	}
	return resourceType
}

func isValidSubresource(resourceType string) bool {
	if strings.Contains(resourceType, "/") {
		parts := strings.Split(resourceType, "/")
		return len(parts) == 2 && parts[1] == "status" // Valid if the subresource is `/status`
	}
	return true // Valid if there’s no subresource
}

func getAPIGroup(apiVersion string) string {
	var apiGroup string
	apiVersionParts := strings.Split(apiVersion, "/")
	if len(apiVersionParts) == 2 {
		apiGroup = apiVersionParts[0]
	} else {
		apiGroup = ""
	}
	return apiGroup
}

func getCacheKey(kind, group string) string {
	if group == "" {
		return kind
	}
	return fmt.Sprintf("%s/%s", kind, group)
}
