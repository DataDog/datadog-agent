// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	refreshing      int32
	refreshDone     chan struct{}
	refreshErr      error
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

	// repopulating the cache on miss.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	return "", fmt.Errorf("resource type not found for kind %q and group %q after refresh", kind, apiGroup)

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	return "", fmt.Errorf("resource kind not found for resource %q and group %q after refresh", resource, apiGroup)
}

func (r *ResourceTypeCache) refreshCache(ctx context.Context) error {
	// check if refresh process is in progress
	if atomic.LoadInt32(&r.refreshing) == 0 {
		// the first go routine to perform CAS will become the leader
		if atomic.CompareAndSwapInt32(&r.refreshing, 0, 1) {

			done := make(chan struct{})

			// Protect setting refreshDone and clearing refreshErr
			r.lock.Lock()
			r.setRefreshDone(done)
			r.refreshErr = nil
			r.lock.Unlock()

			defer func() {

				close(done)
				r.lock.Lock()
				r.setRefreshDone(nil)
				r.lock.Unlock()
				atomic.StoreInt32(&r.refreshing, 0)
			}()

			var refreshRetrier retry.Retrier
			err := refreshRetrier.SetupRetrier(&retry.Config{
				Name: "ResourceTypeCache_refresh_on_miss",
				AttemptMethod: func() error {

					err := r.prepopulateCache()
					if err != nil {
						cacheErr := fmt.Errorf("failed to refresh resource type cache: %w", err)

						r.lock.Lock()
						r.refreshErr = cacheErr
						r.lock.Unlock()
						return cacheErr
					}
					return nil
				},
				Strategy:          retry.Backoff,
				InitialRetryDelay: 200 * time.Millisecond,
				MaxRetryDelay:     2 * time.Second,
			})
			if err != nil {
				cacheErr := fmt.Errorf("failed to initialize refresh retrier: %w", err)
				r.lock.Lock()
				r.refreshErr = cacheErr
				r.lock.Unlock()
				return cacheErr
			}

			for {
				_ = refreshRetrier.TriggerRetry()

				switch refreshRetrier.RetryStatus() {
				case retry.OK:

					r.lock.Lock()
					r.refreshErr = nil
					r.lock.Unlock()
					return nil

				case retry.PermaFail:

					var finalErr error
					if le := refreshRetrier.LastError(); le != nil {
						finalErr = le.Unwrap()
					} else {
						finalErr = fmt.Errorf("permanent failure while refreshing cache")
					}
					r.lock.Lock()
					r.refreshErr = finalErr
					r.lock.Unlock()
					return finalErr

				default:

					sleepFor := time.Until(refreshRetrier.NextRetry())
					if sleepFor < 0 {
						sleepFor = 0
					}
					select {
					case <-ctx.Done():
						finalErr := fmt.Errorf("context deadline reached while waiting to repopulate: %w", ctx.Err())
						r.lock.Lock()
						r.refreshErr = finalErr
						r.lock.Unlock()
						return finalErr
					case <-time.After(sleepFor):

					}
				}
			}
		}
	}

	r.lock.RLock()
	ch := r.getRefreshDone()
	r.lock.RUnlock()

	if ch == nil {
		// small window where leader hasn't set channel yet or already cleared it.

		r.lock.RLock()
		err := r.refreshErr
		r.lock.RUnlock()
		if err != nil {
			return err
		}
		return nil
	}

	// Wait for leader to finish, then read the same outcome
	<-ch
	r.lock.RLock()
	err := r.refreshErr
	r.lock.RUnlock()
	if err != nil {
		return err
	}
	return nil
}

func (r *ResourceTypeCache) getRefreshDone() chan struct{} {
	return r.refreshDone
}

func (r *ResourceTypeCache) setRefreshDone(ch chan struct{}) {
	r.refreshDone = ch
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
