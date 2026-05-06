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

func TestCacheLookupMiss(t *testing.T) {
	c := newCache(time.Now)
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, stateMiss, got.state)
}

func TestCacheLookupHit(t *testing.T) {
	c := newCache(time.Now)
	r := Result{Configs: []integration.Config{{Name: "krakend"}}}
	c.putSuccess("svc-1", "krakend", r)
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, stateHit, got.state)
	assert.Len(t, got.result.Configs, 1)
}

func TestCachePutFailureSchedulesRetries(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	c := newCache(clock)
	schedule := []time.Duration{5 * time.Second, 10 * time.Second}

	c.putFailure("svc-1", "krakend", schedule)
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, statePending, got.state)
	assert.Equal(t, now.Add(5*time.Second), got.nextRetryAt)

	c.putFailure("svc-1", "krakend", schedule)
	got = c.lookup("svc-1", "krakend")
	assert.Equal(t, statePending, got.state)
	assert.Equal(t, now.Add(10*time.Second), got.nextRetryAt)

	c.putFailure("svc-1", "krakend", schedule)
	got = c.lookup("svc-1", "krakend")
	assert.Equal(t, stateGivenUp, got.state)
}

func TestCachePutSuccessClearsFailure(t *testing.T) {
	c := newCache(time.Now)
	c.putFailure("svc-1", "krakend", []time.Duration{5 * time.Second})
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, stateHit, got.state)
}

func TestCacheKeyIsolation(t *testing.T) {
	c := newCache(time.Now)
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	got := c.lookup("svc-1", "apache")
	assert.Equal(t, stateMiss, got.state, "different integration is a different key")
	got = c.lookup("svc-2", "krakend")
	assert.Equal(t, stateMiss, got.state, "different service is a different key")
}

func TestCacheForgetClearsAllEntriesForService(t *testing.T) {
	c := newCache(time.Now)
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	c.putSuccess("svc-1", "apache", Result{Configs: []integration.Config{{Name: "apache"}}})
	c.putSuccess("svc-2", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})

	c.forget("svc-1")

	assert.Equal(t, stateMiss, c.lookup("svc-1", "krakend").state, "svc-1/krakend should be forgotten")
	assert.Equal(t, stateMiss, c.lookup("svc-1", "apache").state, "svc-1/apache should be forgotten")
	assert.Equal(t, stateHit, c.lookup("svc-2", "krakend").state, "svc-2/krakend should be unaffected")
}
