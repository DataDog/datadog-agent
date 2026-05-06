// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestCacheMissReturnsFalse(t *testing.T) {
	c := newCache(time.Now)
	_, _, hit := c.get("svc-1", "krakend")
	assert.False(t, hit)
}

func TestCacheStoresSuccess(t *testing.T) {
	c := newCache(time.Now)
	r := Result{Configs: []integration.Config{{Name: "krakend"}}}
	c.putSuccess("svc-1", "krakend", r)
	got, ok, hit := c.get("svc-1", "krakend")
	assert.True(t, hit)
	assert.True(t, ok)
	assert.Len(t, got.Configs, 1)
}

func TestCacheStoresFailureAndExpires(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	c := newCache(clock)
	c.putFailure("svc-1", "krakend", 30*time.Second)

	_, ok, hit := c.get("svc-1", "krakend")
	assert.True(t, hit)
	assert.False(t, ok, "failure cached")

	now = now.Add(31 * time.Second)
	_, _, hit = c.get("svc-1", "krakend")
	assert.False(t, hit, "failure expired")
}

func TestCacheKeyIsolation(t *testing.T) {
	c := newCache(time.Now)
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	_, _, hit := c.get("svc-1", "apache")
	assert.False(t, hit, "different integration is a different key")
	_, _, hit = c.get("svc-2", "krakend")
	assert.False(t, hit, "different service is a different key")
}
