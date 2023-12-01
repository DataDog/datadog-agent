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

	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultExpire = 3 * time.Minute
	defaultPurge  = 30 * time.Second

	// ClusterAgeCacheKey is the key name for the orchestrator cluster age in the agent in-mem cache
	ClusterAgeCacheKey = "orchestratorClusterAge"
)

var (
	// cache hit
	cacheExpVars = expvar.NewMap("orchestrator-cache")
	cacheHits    = map[pkgorchestratormodel.NodeType]*expvar.Int{}

	// cache miss, send to backend
	sendExpVars = expvar.NewMap("orchestrator-sends")
	cacheMiss   = map[pkgorchestratormodel.NodeType]*expvar.Int{}

	// KubernetesResourceCache provides an in-memory key:value store similar to memcached for kubernetes resources.
	KubernetesResourceCache = cache.New(defaultExpire, defaultPurge)

	// Telemetry
	tlmCacheHits   = telemetry.NewCounter("orchestrator", "cache_hits", []string{"orchestrator", "resource"}, "Number of cache hits")
	tlmCacheMisses = telemetry.NewCounter("orchestrator", "cache_misses", []string{"orchestrator", "resource"}, "Number of cache misses")
)

func init() {
	for _, nodeType := range pkgorchestratormodel.NodeTypes() {
		cacheHits[nodeType] = &expvar.Int{}
		cacheMiss[nodeType] = &expvar.Int{}
		cacheExpVars.Set(nodeType.String(), cacheHits[nodeType])
		sendExpVars.Set(nodeType.String(), cacheMiss[nodeType])
	}
}

// SkipKubernetesResource checks with a global kubernetes cache whether the resource was already reported.
// It will return true in case the UID is in the cache and the resourceVersion did not change. Else it will return false.
// 0 == defaultDuration
func SkipKubernetesResource(uid types.UID, resourceVersion string, nodeType pkgorchestratormodel.NodeType) bool {
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

func incCacheHit(nodeType pkgorchestratormodel.NodeType) {
	if nodeType.String() == "" {
		log.Errorf("Unknown NodeType %v will not update cache hits", nodeType)
		return
	}
	e := cacheHits[nodeType]
	e.Add(1)
	tlmCacheHits.Inc(nodeType.TelemetryTags()...)
}

func incCacheMiss(nodeType pkgorchestratormodel.NodeType) {
	if nodeType.String() == "" {
		log.Errorf("Unknown NodeType %v will not update cache misses", nodeType)
		return
	}
	e := cacheMiss[nodeType]
	e.Add(1)
	tlmCacheMisses.Inc(nodeType.TelemetryTags()...)
}
