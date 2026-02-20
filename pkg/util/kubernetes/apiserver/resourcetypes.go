// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"errors"
	"fmt"
	"maps"
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

type ClusterResource struct {
	Group      string
	APIVersion string
	Kind       string
}

// ResourceTypeCache is a global cache to store Kubernetes resource types.
type ResourceTypeCache struct {
	// kindGroupToType maps a resource kind (singular name) and api group to resource type (plural name)
	// this mapping is assumed bijective.
	kindGroupToType map[string]string
	// typeGroupToKind is a reverse map of kindGroupToType
	typeGroupToKind  map[string]string
	clusterResources map[string]ClusterResource
	lock             sync.RWMutex
	discoveryClient  discovery.DiscoveryInterface
	refreshing       bool
	refreshWaitCh    chan struct{}
	refreshErr       error
	refreshTTL       time.Duration
}

// InitializeGlobalResourceTypeCache initializes the global cache if it hasn't been already.
func InitializeGlobalResourceTypeCache(discoveryClient discovery.DiscoveryInterface) error {
	cacheOnce.Do(func() {
		resourceCache = &ResourceTypeCache{
			kindGroupToType:  make(map[string]string),
			typeGroupToKind:  make(map[string]string),
			clusterResources: make(map[string]ClusterResource),
			discoveryClient:  discoveryClient,
			refreshTTL:       5 * time.Second,
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
		return "", errors.New("resource type cache is not initialized")
	}
	return resourceCache.getResourceType(kind, group)
}

// GetResourceKind retrieves the kind given the resource plural name and group.
func GetResourceKind(resource, apiGroup string) (string, error) {
	if resourceCache == nil {
		return "", errors.New("resource type cache is not initialized")
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

// GetAPIVersion extracts the version from an API version string (e.g., "apps/v1" → "v1").
func GetAPIVersion(apiVersion string) string {
	if index := strings.Index(apiVersion, "/"); index > 0 {
		return apiVersion[index+1:]
	}
	return apiVersion
}

// GetClusterResources return all known resources from the cache
func GetClusterResources() (map[string]ClusterResource, error) {
	if resourceCache == nil {
		return nil, errors.New("resource type cache is not initialized")
	}
	return resourceCache.getClusterResources(), nil
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

	// repopulating the cache on miss.
	ctx, cancel := context.WithTimeout(context.Background(), r.refreshTTL)
	defer cancel()

	err := r.refreshCache(ctx)

	if err != nil {
		return "", err
	}

	r.lock.RLock()
	resourceType, found = r.kindGroupToType[cacheKey]
	r.lock.RUnlock()

	if found {
		return resourceType, nil
	}
	return "", fmt.Errorf("resource type not found for kind %q and group %q", kind, apiGroup)

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

	// repopulating the cache on miss.
	ctx, cancel := context.WithTimeout(context.Background(), r.refreshTTL)
	defer cancel()

	err := r.refreshCache(ctx)

	if err != nil {
		return "", err
	}

	r.lock.RLock()
	kind, found = r.typeGroupToKind[cacheKey]
	r.lock.RUnlock()

	if found {
		return kind, nil
	}
	return "", fmt.Errorf("resource kind not found for resource %q and group %q", resource, apiGroup)
}

func (r *ResourceTypeCache) getClusterResources() map[string]ClusterResource {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return maps.Clone(r.clusterResources)
}

func (r *ResourceTypeCache) refreshCache(ctx context.Context) error {
	// decide leader vs follower under the lock.
	r.lock.Lock()
	if r.refreshing {
		// leader is already refreshing
		waitCh := r.refreshWaitCh
		r.lock.Unlock()

		// wait for refresh to complete
		select {
		case <-waitCh:
			r.lock.RLock()
			err := r.refreshErr
			r.lock.RUnlock()
			return err
		case <-ctx.Done():
			return fmt.Errorf("context deadline reached while waiting to repopulate: %w", ctx.Err())
		}

	}

	// no refresh in progress -> set state and create wait channel
	r.refreshing = true
	waitCh := make(chan struct{})
	r.refreshWaitCh = waitCh
	r.refreshErr = nil
	r.lock.Unlock()

	// ensure followers are released
	var runErr error
	defer func() {
		r.lock.Lock()
		r.refreshErr = runErr
		close(r.refreshWaitCh)
		r.refreshWaitCh = nil
		r.refreshing = false
		r.lock.Unlock()
	}()

	// retry config for refreshing the cache
	var refreshRetrier retry.Retrier
	if err := refreshRetrier.SetupRetrier(&retry.Config{
		Name: "ResourceTypeCache_refresh",
		AttemptMethod: func() error {

			if err := r.prepopulateCache(); err != nil {
				return fmt.Errorf("failed to refresh resource type cache: %w", err)
			}
			return nil
		},
		Strategy:          retry.Backoff,
		InitialRetryDelay: 200 * time.Millisecond,
		MaxRetryDelay:     2 * time.Second,
	}); err != nil {
		runErr = fmt.Errorf("failed to initialize refresh retrier: %w", err)
		return runErr
	}

	// retry method for refreshing the cache
	for {
		_ = refreshRetrier.TriggerRetry()
		status := refreshRetrier.RetryStatus()

		switch status {
		case retry.OK:
			return nil
		case retry.PermaFail:
			le := refreshRetrier.LastError()
			runErr = le.Unwrap()
			return runErr

		}

		// wait until next retry or context timeout.
		sleepFor := time.Until(refreshRetrier.NextRetry())
		if sleepFor < 0 {
			sleepFor = 0
		}

		select {
		case <-ctx.Done():
			runErr = fmt.Errorf("context deadline reached while waiting to repopulate: %w", ctx.Err())
			return runErr
		case <-time.After(sleepFor):

		}
	}
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

			if !resource.Namespaced {
				r.clusterResources[trimmedResourceType] = ClusterResource{
					Group:      group,
					APIVersion: GetAPIVersion(list.GroupVersion),
					Kind:       kind,
				}
			}
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
	return resource + "/" + group
}
