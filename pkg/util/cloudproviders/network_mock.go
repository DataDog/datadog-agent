// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package cloudproviders

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// MockNetworkID mock the answer returned by GetNetworkID by setting a value in the cache. This value will be removed
// from the cache during the test cleanup phase.
func MockNetworkID(t *testing.T, networkID string) {
	t.Cleanup(func() { cache.Cache.Delete(networkIDCacheKey) })
	cache.Cache.Set(networkIDCacheKey, networkID, cache.NoExpiration)
}
