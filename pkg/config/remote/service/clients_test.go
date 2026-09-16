// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

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

// TestClientsSeenActiveClientsRace catches concurrent mutation of a *pbgo.Client shared via seen()/activeClients(); run with -race.
func TestClientsSeenActiveClientsRace(t *testing.T) {
	testTTL := time.Second * 5
	realClock := clock.New()
	clients := newClients(realClock, testTTL)

	// Shared *pbgo.Client, mimicking a reused request.Client.
	pbClient := &pbgo.Client{
		Id:       "client1",
		Products: []string{"APM_SAMPLING"},
	}

	const iterations = 2000
	var wg sync.WaitGroup
	// Repeatedly mark the client as seen, like ClientGetConfigs does.
	wg.Go(func() {
		for i := 0; i < iterations; i++ {
			clients.seen(pbClient)
		}
	})

	// Repeatedly fetch and marshal active clients, like refresh() does.
	wg.Go(func() {
		for i := 0; i < iterations; i++ {
			for _, active := range clients.activeClients() {
				if _, err := proto.Marshal(active); err != nil {
					t.Errorf("failed to marshal client: %v", err)
				}
			}
		}
	})

	wg.Wait()
}
