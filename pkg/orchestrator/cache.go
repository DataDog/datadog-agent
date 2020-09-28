// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"expvar"
	"time"

	cache "github.com/patrickmn/go-cache"

	"k8s.io/apimachinery/pkg/types"
)

const (
	defaultExpire = 3 * time.Minute
	defaultPurge  = 30 * time.Second
)

// TODO: make it possible to find information about single workloads

var (
	orchestratorCacheExpVars = expvar.NewMap("orchestrator-cache")
	DeploymentCacheHits      = expvar.Int{}
	ReplicaSetCacheHits      = expvar.Int{}
	NodeCacheHits            = expvar.Int{}
	ServiceCacheHits         = expvar.Int{}
	PodCacheHits             = expvar.Int{}

	KubernetesResourceCache = cache.New(defaultExpire, defaultPurge)
)

func init() {
	orchestratorCacheExpVars.Set("PodsCacheHits", &PodCacheHits)
	orchestratorCacheExpVars.Set("DeploymentCacheHits", &DeploymentCacheHits)
	orchestratorCacheExpVars.Set("ReplicaSetsCacheHits", &ReplicaSetCacheHits)
	orchestratorCacheExpVars.Set("NodeCacheHits", &NodeCacheHits)
	orchestratorCacheExpVars.Set("ServiceCacheHits", &ServiceCacheHits)
}

// KubernetesResourceCache provides an in-memory key:value store similar to memcached for kubernetes resources.
var (
	KubernetesResourceDeploymentCache = cache.New(defaultExpire, defaultPurge)
	KubernetesResourceReplicaSetCache = cache.New(defaultExpire, defaultPurge)
	KubernetesResourcePodCache        = cache.New(defaultExpire, defaultPurge)
	KubernetesResourceNodeCache       = cache.New(defaultExpire, defaultPurge)
)

// SkipKubernetesResource checks with a global kubernetes cache whether the resource was already reported.
// It will return true in case the UID is in the cache and the resourceVersion did not change. Else it will return false.
// 0 == defaultDuration
func SkipKubernetesResource(uid types.UID, resourceVersion string) bool {
	cacheKey := string(uid)
	value, hit := KubernetesResourceCache.Get(cacheKey)

	if !hit {
		KubernetesResourceCache.Set(cacheKey, resourceVersion, 0)
		return false
	}
	if value != resourceVersion {
		KubernetesResourceCache.Set(cacheKey, resourceVersion, 0)
		return false
	}
	return true
}
