// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/benbjohnson/clock"
)

type client struct {
	lastSeen time.Time
	pbClient *pbgo.Client
}

func (c *client) expired(clock clock.Clock, ttl time.Duration) bool {
	return clock.Now().After(c.lastSeen.Add(ttl))
}

type clients struct {
	m     sync.Mutex
	clock clock.Clock

	clientsTTL time.Duration
	clients    map[string]*client
}

func newClients(clock clock.Clock, clientsTTL time.Duration) *clients {
	return &clients{
		clock:      clock,
		clientsTTL: clientsTTL,
		clients:    make(map[string]*client),
	}
}

// seen marks the given client as active
func (c *clients) seen(pbClient *pbgo.Client) {
	c.m.Lock()
	defer c.m.Unlock()
	now := c.clock.Now().UTC()
	pbClient.LastSeen = uint64(now.UnixMilli())
	c.clients[pbClient.Id] = &client{
		lastSeen: now,
		pbClient: pbClient,
	}
}

// activeClients returns the list of active clients
func (c *clients) activeClients() []*pbgo.Client {
	c.m.Lock()
	defer c.m.Unlock()
	var activeClients []*pbgo.Client
	for id, client := range c.clients {
		if client.expired(c.clock, c.clientsTTL) {
			delete(c.clients, id)
			continue
		}
		activeClients = append(activeClients, client.pbClient)
	}
	return activeClients
}
