// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"expvar"
	"time"

	"github.com/patrickmn/go-cache"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultExpire = 3 * time.Minute
	defaultPurge  = 30 * time.Second
	// NoExpiration maps to go-cache corresponding value
	NoExpiration = cache.NoExpiration
)

var (
	cacheExpVars        = expvar.NewMap("orchestrator-cache")
	deploymentCacheHits = expvar.Int{}
	replicaSetCacheHits = expvar.Int{}
	nodeCacheHits       = expvar.Int{}
	serviceCacheHits    = expvar.Int{}
	podCacheHits        = expvar.Int{}

	sendExpVars    = expvar.NewMap("orchestrator-sends")
	deploymentHits = expvar.Int{}
	replicaSetHits = expvar.Int{}
	nodeHits       = expvar.Int{}
	serviceHits    = expvar.Int{}
	podHits        = expvar.Int{}

	// KubernetesResourceCache provides an in-memory key:value store similar to memcached for kubernetes resources.
	KubernetesResourceCache = cache.New(defaultExpire, defaultPurge)
)

func init() {
	cacheExpVars.Set("Pods", &podCacheHits)
	cacheExpVars.Set("Deployments", &deploymentCacheHits)
	cacheExpVars.Set("ReplicaSets", &replicaSetCacheHits)
	cacheExpVars.Set("Nodes", &nodeCacheHits)
	cacheExpVars.Set("Services", &serviceCacheHits)

	sendExpVars.Set("Pods", &podHits)
	sendExpVars.Set("Deployments", &deploymentHits)
	sendExpVars.Set("ReplicaSets", &replicaSetHits)
	sendExpVars.Set("Nodes", &nodeHits)
	sendExpVars.Set("Services", &serviceHits)
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
		nodeCacheHits.Add(1)
	case K8sService:
		serviceCacheHits.Add(1)
	case K8sReplicaSet:
		replicaSetCacheHits.Add(1)
	case K8sDeployment:
		deploymentCacheHits.Add(1)
	case K8sPod:
		podCacheHits.Add(1)
	default:
		log.Errorf("Cannot increment unknown nodeType, iota: %v", nodeType)
	}
}

func incCacheMiss(nodeType NodeType) {
	switch nodeType {
	case K8sNode:
		nodeHits.Add(1)
	case K8sService:
		serviceHits.Add(1)
	case K8sReplicaSet:
		replicaSetHits.Add(1)
	case K8sDeployment:
		deploymentHits.Add(1)
	case K8sPod:
		podHits.Add(1)
	default:
		log.Errorf("Cannot increment unknown nodeType, iota: %v", nodeType)
	}
}
