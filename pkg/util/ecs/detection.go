// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/common"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	hasFargateResourceTagsCacheKey = "HasFargateResourceTagsCacheKey"
	hasEC2ResourceTagsCacheKey     = "HasEC2ResourceTagsCacheKey"
	hasEC2ResourceTagsCacheExpiry  = 5 * time.Minute
)

// HasEC2ResourceTags returns whether the metadata endpoint in ECS exposes
// resource tags.
func HasEC2ResourceTags() bool {
	if !config.IsCloudProviderEnabled(common.CloudProviderName) {
		return false
	}
	return queryCacheBool(hasEC2ResourceTagsCacheKey, func() (bool, time.Duration) {
		client, err := ecsmeta.V3orV4FromCurrentTask()
		if err != nil {
			log.Debugf("failed to detect V3 and V4 metadata endpoint: %s", err)
			return false, hasEC2ResourceTagsCacheExpiry
		}
		_, err = client.GetTaskWithTags(context.TODO())
		if err != nil {
			log.Debugf("failed to get task with tags: %s", err)
		}
		return err == nil, hasEC2ResourceTagsCacheExpiry
	})
}

// HasFargateResourceTags returns whether the metadata endpoint in Fargate
// exposes resource tags.
func HasFargateResourceTags(ctx context.Context) bool {
	return queryCacheBool(hasFargateResourceTagsCacheKey, func() (bool, time.Duration) {
		client, err := ecsmeta.V2()
		if err != nil {
			log.Debugf("error while initializing ECS metadata V2 client: %s", err)
			return newBoolEntry(false)
		}

		_, err = client.GetTaskWithTags(ctx)
		return newBoolEntry(err == nil)
	})
}

func queryCacheBool(cacheKey string, cacheMissEvalFunc func() (bool, time.Duration)) bool {
	if !config.IsCloudProviderEnabled(common.CloudProviderName) {
		return false
	}
	if cachedValue, found := cache.Cache.Get(cacheKey); found {
		if v, ok := cachedValue.(bool); ok {
			return v
		}
		log.Errorf("Invalid cache format for key %q: forcing a cache miss", cacheKey)
	}

	newValue, ttl := cacheMissEvalFunc()
	cache.Cache.Set(cacheKey, newValue, ttl)

	return newValue
}

func newBoolEntry(v bool) (bool, time.Duration) {
	if v == true {
		return v, 5 * time.Minute
	}
	return v, cache.NoExpiration
}
