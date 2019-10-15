// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package ecs

import (
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	isFargateInstanceCacheKey      = "IsFargateInstanceCacheKey"
	hasFargateResourceTagsCacheKey = "HasFargateResourceTagsCacheKey"
	hasEC2ResourceTagsCacheKey     = "HasEC2ResourceTagsCacheKey"

	// CloudProviderName contains the inventory name of for ECS
	CloudProviderName = "AWS"
)

// IsECSInstance returns whether the agent is running in ECS.
func IsECSInstance() bool {
	_, err := ecsmeta.V1()
	return err == nil
}

// IsFargateInstance returns whether the agent is in an ECS fargate task.
// It detects it by getting and unmarshalling the metadata API response.
func IsFargateInstance() bool {
	return queryCacheBool(isFargateInstanceCacheKey, func() (bool, time.Duration) {

		// This envvar is set to AWS_ECS_EC2 on classic EC2 instances
		// Versions 1.0.0 to 1.3.0 (latest at the time) of the Fargate
		// platform set this envvar.
		// If Fargate detection were to fail, running a container with
		// `env` as cmd will allow to check if it is still present.
		if os.Getenv("AWS_EXECUTION_ENV") != "AWS_ECS_FARGATE" {
			return newBoolEntry(false)
		}

		_, err := ecsmeta.V2().GetTask()
		if err != nil {
			log.Debug(err)
			return newBoolEntry(false)
		}

		return newBoolEntry(true)
	})
}

// IsRunningOn returns true if the agent is running on ECS/Fargate
func IsRunningOn() bool {
	return IsECSInstance() || IsFargateInstance()
}

// HasEC2ResourceTags returns whether the metadata endpoint in ECS exposes
// resource tags.
func HasEC2ResourceTags() bool {
	return queryCacheBool(hasEC2ResourceTagsCacheKey, func() (bool, time.Duration) {
		client, err := ecsmeta.V3FromCurrentTask()
		if err != nil {
			return newBoolEntry(false)
		}
		_, err = client.GetTaskWithTags()
		return newBoolEntry(err == nil)
	})
}

// HasFargateResourceTags returns whether the metadata endpoint in Fargate
// exposes resource tags.
func HasFargateResourceTags() bool {
	return queryCacheBool(hasFargateResourceTagsCacheKey, func() (bool, time.Duration) {
		_, err := ecsmeta.V2().GetTaskWithTags()
		return newBoolEntry(err == nil)
	})
}

func queryCacheBool(cacheKey string, cacheMissEvalFunc func() (bool, time.Duration)) bool {
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
