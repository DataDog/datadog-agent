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
	expireAt time.Time
	pbClient *pbgo.Client
}

func (c *client) expired(clock clock.Clock) bool {
	return clock.Now().After(c.expireAt)
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
	c.clients[pbClient.Id] = &client{
		expireAt: c.clock.Now().Add(c.clientsTTL),
		pbClient: pbClient,
	}
}

// activeClients returns the list of active clients
func (c *clients) activeClients() []*pbgo.Client {
	c.m.Lock()
	defer c.m.Unlock()
	var activeClients []*pbgo.Client
	for id, client := range c.clients {
		if client.expired(c.clock) {
			delete(c.clients, id)
			continue
		}
		activeClients = append(activeClients, client.pbClient)
	}
	return activeClients
}
