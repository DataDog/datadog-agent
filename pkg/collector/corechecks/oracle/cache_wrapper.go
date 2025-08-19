// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"time"

	cache "github.com/patrickmn/go-cache"
)

func newCache(retention int) *cache.Cache {
	return cache.New(time.Duration(retention)*time.Minute, 10*time.Minute)
}

func getFqtEmittedCache() *cache.Cache {
	return newCache(60)
}

func getPlanEmittedCache(c *Check) *cache.Cache {
	var planCacheRetention = c.config.ExecutionPlans.PlanCacheRetention
	if planCacheRetention == 0 {
		planCacheRetention = 1
	}

	return newCache(planCacheRetention)
}
