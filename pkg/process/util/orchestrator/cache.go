// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"time"

	cache "github.com/patrickmn/go-cache"

	"k8s.io/apimachinery/pkg/types"
)

const (
	defaultExpire = 3 * time.Minute
	defaultPurge  = 30 * time.Second
)

// KubernetesResourceCache provides an in-memory key:value store similar to memcached for kubernetes resources.
var KubernetesResourceCache = cache.New(defaultExpire, defaultPurge)

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
