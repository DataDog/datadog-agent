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

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
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

func TestNewActiveClientsRateLimit(t *testing.T) {
	clock := clock.NewMock()
	newActiveClients := newActiveClients{
		clock:    clock,
		requests: make(chan chan struct{}),
		until:    clock.Now(),
	}

	newActiveClients.setRateLimit(time.Hour)
	assert.Equal(t, clock.Now().UTC().Add(defaultClientsTTL), newActiveClients.until)
	newActiveClients.setRateLimit(5 * time.Second)
	assert.Equal(t, clock.Now().UTC().Add(5*time.Second), newActiveClients.until)
	newActiveClients.setRateLimit(time.Second)
	assert.Equal(t, clock.Now().UTC().Add(defaultClientsTTL), newActiveClients.until)
}
