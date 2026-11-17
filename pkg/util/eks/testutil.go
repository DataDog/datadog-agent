// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package eks

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// CacheFailureForTest puts err in the real identity cache for clusterName and clusterCA, as a
// failed DescribeCluster would, expiring in ttl. GetClusterIdentity then serves it, and RetryIn
// reports its expiry, so tests in other packages can drive the real cache.
func CacheFailureForTest(t testing.TB, clusterName string, clusterCA []byte, err error, ttl time.Duration) {
	t.Helper()
	_, cacheKey := identityKeys(clusterName, clusterCA)
	cache.Cache.Set(cacheKey, &cachedFailure{err: err, expires: time.Now().Add(ttl)}, ttl)
	t.Cleanup(func() { cache.Cache.Delete(cacheKey) })
}
