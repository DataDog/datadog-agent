// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"expvar"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	CacheExpVars        = expvar.NewMap("orchestrator-cache")
	DeploymentCacheHits = expvar.Int{}
	ReplicaSetCacheHits = expvar.Int{}
	NodeCacheHits       = expvar.Int{}
	ServiceCacheHits    = expvar.Int{}
	PodCacheHits        = expvar.Int{}

	SendExpVars    = expvar.NewMap("orchestrator-sends")
	DeploymentHits = expvar.Int{}
	ReplicaSetHits = expvar.Int{}
	NodeHits       = expvar.Int{}
	ServiceHits    = expvar.Int{}
	PodHits        = expvar.Int{}
)

func init() {
	CacheExpVars.Set("PodsCacheHits", &PodCacheHits)
	CacheExpVars.Set("DeploymentCacheHits", &DeploymentCacheHits)
	CacheExpVars.Set("ReplicaSetsCacheHits", &ReplicaSetCacheHits)
	CacheExpVars.Set("NodeCacheHits", &NodeCacheHits)
	CacheExpVars.Set("ServiceCacheHits", &ServiceCacheHits)

	SendExpVars.Set("Pods", &PodHits)
	SendExpVars.Set("Deployment", &DeploymentHits)
	SendExpVars.Set("ReplicaSets", &ReplicaSetHits)
	SendExpVars.Set("Node", &NodeHits)
	SendExpVars.Set("Service", &ServiceHits)
}

// KubernetesResourceCache provides an in-memory key:value store similar to memcached for kubernetes resources.
var (
	KubernetesResourceDeploymentCache = cache.New(defaultExpire, defaultPurge)
	KubernetesResourceReplicaSetCache = cache.New(defaultExpire, defaultPurge)
	KubernetesResourcePodCache        = cache.New(defaultExpire, defaultPurge)
	KubernetesResourceNodeCache       = cache.New(defaultExpire, defaultPurge)
	KubernetesResourceServiceCache    = cache.New(defaultExpire, defaultPurge)
)

// SkipKubernetesResource checks with a global kubernetes cache whether the resource was already reported.
// It will return true in case the UID is in the cache and the resourceVersion did not change. Else it will return false.
// 0 == defaultDuration
func SkipKubernetesResource(uid types.UID, resourceVersion string, nodeType NodeType) bool {
	cacheKey := string(uid)
	kubernetesResourceCache := getNodeTypeCache(nodeType)
	value, hit := kubernetesResourceCache.Get(cacheKey)

	if !hit {
		kubernetesResourceCache.Set(cacheKey, resourceVersion, 0)
		incCacheMiss(nodeType)
		return false
	} else if value != resourceVersion {
		incCacheMiss(nodeType)
		kubernetesResourceCache.Set(cacheKey, resourceVersion, 0)
		return false
	} else {
		incCacheHit(nodeType)
		return true
	}
}

// TODO: Refactor orchestrator related packaged to reduce those switch cases by using interfaces and types.
func getNodeTypeCache(nodeType NodeType) *cache.Cache {
	switch nodeType {
	case K8sNode:
		return KubernetesResourceNodeCache
	case K8sService:
		return KubernetesResourceServiceCache
	case K8sReplicaSet:
		return KubernetesResourceReplicaSetCache
	case K8sDeployment:
		return KubernetesResourceDeploymentCache
	case K8sPod:
		return KubernetesResourcePodCache
	default:
		log.Errorf("Cannot get cache of unknown nodeType, iota: %v", nodeType)
	}
	return nil
}

func incCacheHit(nodeType NodeType) {
	switch nodeType {
	case K8sNode:
		NodeCacheHits.Add(1)
	case K8sService:
		ServiceCacheHits.Add(1)
	case K8sReplicaSet:
		ReplicaSetCacheHits.Add(1)
	case K8sDeployment:
		DeploymentCacheHits.Add(1)
	case K8sPod:
		PodCacheHits.Add(1)
	default:
		log.Errorf("Cannot increment unknown nodeType, iota: %v", nodeType)
	}
}

func incCacheMiss(nodeType NodeType) {
	switch nodeType {
	case K8sNode:
		NodeHits.Add(1)
	case K8sService:
		ServiceHits.Add(1)
	case K8sReplicaSet:
		ReplicaSetHits.Add(1)
	case K8sDeployment:
		DeploymentHits.Add(1)
	case K8sPod:
		PodHits.Add(1)
	default:
		log.Errorf("Cannot increment unknown nodeType, iota: %v", nodeType)
	}
}
