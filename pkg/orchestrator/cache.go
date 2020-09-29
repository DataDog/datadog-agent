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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultExpire = 3 * time.Minute
	defaultPurge  = 30 * time.Second
	// NoExpiration maps to go-cache corresponding value
	NoExpiration = cache.NoExpiration
)

// TODO: make it possible to find information about single workloads

// TODO: add efficiency per run e.g.
/**
  ======================
  Cache Stats (Last Run)
  ======================
    Pods: 15 (30 cached)
total pods send: 110000 Pods send
total pods cache hits: 200000 cache hits
total cache size: len(cache)
*/

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

	// KubernetesResourceCache provides an in-memory key:value store similar to memcached for kubernetes resources.
	KubernetesResourceCache = cache.New(defaultExpire, defaultPurge)
)

func init() {
	CacheExpVars.Set("Pods", &PodCacheHits)
	CacheExpVars.Set("Deployments", &DeploymentCacheHits)
	CacheExpVars.Set("ReplicaSets", &ReplicaSetCacheHits)
	CacheExpVars.Set("Nodes", &NodeCacheHits)
	CacheExpVars.Set("Services", &ServiceCacheHits)

	SendExpVars.Set("Pods", &PodHits)
	SendExpVars.Set("Deployments", &DeploymentHits)
	SendExpVars.Set("ReplicaSets", &ReplicaSetHits)
	SendExpVars.Set("Nodes", &NodeHits)
	SendExpVars.Set("Services", &ServiceHits)
}


// SkipKubernetesResource checks with a global kubernetes cache whether the resource was already reported.
// It will return true in case the UID is in the cache and the resourceVersion did not change. Else it will return false.
// 0 == defaultDuration
func SkipKubernetesResource(uid types.UID, resourceVersion string, nodeType NodeType) bool {
	cacheKey := string(uid)
	value, hit := KubernetesResourceCache.Get(cacheKey)

	if !hit {
		KubernetesResourceCache.Set(cacheKey, resourceVersion, 0)
		incCacheMiss(nodeType)
		return false
	} else if value != resourceVersion {
		incCacheMiss(nodeType)
		KubernetesResourceCache.Set(cacheKey, resourceVersion, 0)
		return false
	} else {
		incCacheHit(nodeType)
		return true
	}
}


// TODO introduce proper interface and typing between different resources.
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
