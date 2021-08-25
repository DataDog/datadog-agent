// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package ecs

import (
	"context"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/common"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	isFargateInstanceCacheKey      = "IsFargateInstanceCacheKey"
	hasFargateResourceTagsCacheKey = "HasFargateResourceTagsCacheKey"
	hasEC2ResourceTagsCacheKey     = "HasEC2ResourceTagsCacheKey"
	hasEC2ResourceTagsCacheExpiry  = 5 * time.Minute
)

// IsECSInstance returns whether the agent is running in ECS.
func IsECSInstance() bool {
	if !config.IsCloudProviderEnabled(common.CloudProviderName) {
		return false
	}
	_, err := ecsmeta.V1()
	return err == nil
}

// IsFargateInstance returns whether the agent is in an ECS fargate task.
// It detects it by getting and unmarshalling the metadata API response.
// This function identifies Fargate on ECS only. Make sure to use the Fargate pkg
// to identify Fargate instances in other orchestrators (e.g EKS Fargate)
func IsFargateInstance(ctx context.Context) bool {
	if !config.IsCloudProviderEnabled(common.CloudProviderName) {
		return false
	}
	return queryCacheBool(isFargateInstanceCacheKey, func() (bool, time.Duration) {

		// This envvar is set to AWS_ECS_EC2 on classic EC2 instances
		// Versions 1.0.0 to 1.3.0 (latest at the time) of the Fargate
		// platform set this envvar.
		// If Fargate detection were to fail, running a container with
		// `env` as cmd will allow to check if it is still present.
		if os.Getenv("AWS_EXECUTION_ENV") != "AWS_ECS_FARGATE" {
			return newBoolEntry(false)
		}

		client, err := ecsmeta.V2()
		if err != nil {
			log.Debugf("error while initializing ECS metadata V2 client: %s", err)
			return newBoolEntry(false)
		}

		_, err = client.GetTask(ctx)
		if err != nil {
			log.Debug(err)
			return newBoolEntry(false)
		}

		return newBoolEntry(true)
	})
}

// IsRunningOn returns true if the agent is running on ECS/Fargate
func IsRunningOn(ctx context.Context) bool {
	return IsECSInstance() || IsFargateInstance(ctx)
}

// HasEC2ResourceTags returns whether the metadata endpoint in ECS exposes
// resource tags.
func HasEC2ResourceTags() bool {
	if !config.IsCloudProviderEnabled(common.CloudProviderName) {
		return false
	}
	return queryCacheBool(hasEC2ResourceTagsCacheKey, func() (bool, time.Duration) {
		client, err := ecsmeta.V3FromCurrentTask()
		if err != nil {
			log.Debugf("failed to detect V3 metadata endpoint: %s", err)
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

// GetNTPHosts returns the NTP hosts for ECS/Fargate if it is detected as the cloud provider, otherwise an empty array.
// Docs: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/set-time.html#configure_ntp
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{"169.254.169.123"}
	}

	return nil
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
