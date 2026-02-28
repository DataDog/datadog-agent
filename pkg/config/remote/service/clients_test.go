// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestClients(t *testing.T) {
	testTTL := time.Second * 5
	clock := clock.NewMock()
	clients := newClients(clock, testTTL)
	testClient1 := &pbgo.Client{
		Id: "client1",
	}
	testClient2 := &pbgo.Client{
		Id: "client2",
	}
	clock.Set(time.Now()) // 0s
	clients.seen(testClient1)
	clock.Add(time.Second * 4) // 4s
	clients.seen(testClient2)
	assert.ElementsMatch(t, []*pbgo.Client{testClient1, testClient2}, clients.activeClients())
	clock.Add(time.Second * 3) // 7s
	assert.ElementsMatch(t, []*pbgo.Client{testClient2}, clients.activeClients())
	clock.Add(time.Second*2 + 1*time.Nanosecond) // 10s1ns
	assert.ElementsMatch(t, []*pbgo.Client{}, clients.activeClients())
	assert.Empty(t, clients.clients)
}

func TestHasNewProducts(t *testing.T) {
	testTTL := time.Second * 30
	clock := clock.NewMock()
	clock.Set(time.Now())
	clients := newClients(clock, testTTL)

	// A client that has never been seen has no new products (hasNewProducts
	// returns false for unknown clients — the new-client bypass handles that case).
	unknownClient := &pbgo.Client{
		Id:       "unknown",
		Products: []string{"APM_TRACING"},
	}
	assert.False(t, clients.hasNewProducts(unknownClient))

	// Register a client with APM_TRACING
	client := &pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_TRACING"},
	}
	clients.seen(client)

	// Same products → no new products
	assert.False(t, clients.hasNewProducts(&pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_TRACING"},
	}))

	// Adding FFE_FLAGS → has new products
	assert.True(t, clients.hasNewProducts(&pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_TRACING", "FFE_FLAGS"},
	}))

	// After seen() with the new product set, no longer has new products
	clients.seen(&pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_TRACING", "FFE_FLAGS"},
	})
	assert.False(t, clients.hasNewProducts(&pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_TRACING", "FFE_FLAGS"},
	}))

	// Removing a product is NOT considered a "new product" — no bypass needed
	assert.False(t, clients.hasNewProducts(&pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_TRACING"},
	}))

	// Adding a different new product triggers again
	assert.True(t, clients.hasNewProducts(&pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_TRACING", "FFE_FLAGS", "LIVE_DEBUGGING"},
	}))
}

func TestCacheBypassClientsRateLimit(t *testing.T) {
	clock := clock.NewMock()
	cacheBypassClients := rateLimiter{
		clock:         clock,
		currentWindow: clock.Now(),
		// Allows 3 bypass every 5 seconds
		windowDuration: 5 * time.Second,
		capacity:       3,
		allowance:      3,
	}

	// 3 bypass
	assert.False(t, cacheBypassClients.Limit())
	assert.False(t, cacheBypassClients.Limit())
	assert.False(t, cacheBypassClients.Limit())
	// bypass blocked
	assert.True(t, cacheBypassClients.Limit())
	assert.True(t, cacheBypassClients.Limit())

	// Still blocked after 2 seconds, since we'rer still in the fixed window
	clock.Add(2 * time.Second)
	assert.True(t, cacheBypassClients.Limit())

	// New window
	clock.Add(4 * time.Second)
	assert.False(t, cacheBypassClients.Limit())
}
