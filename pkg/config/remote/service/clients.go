// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type client struct {
	lastSeen time.Time
	pbClient *pbgo.Client
}

// rateLimiter limits the number of requests that can be made in a given window.
type rateLimiter struct {
	clock clock.Clock

	// Fixed window rate limiting
	// It allows client requests spikes while limiting the global amount of request
	currentWindow  time.Time
	windowDuration time.Duration
	capacity       int
	allowance      int
}

func (c *rateLimiter) Limit() bool {
	now := c.clock.Now()
	window := now.Truncate(c.windowDuration)

	// If we're in a new window, reset the allowance
	if c.currentWindow != window {
		c.currentWindow = window
		c.allowance = c.capacity
	}

	if c.allowance <= 0 {
		// If the window is already overflowed limit the request
		return true
	}
	c.allowance--
	return false
}

func (c *client) expired(clock clock.Clock, ttl time.Duration) bool {
	return clock.Now().UTC().After(c.lastSeen.Add(ttl))
}

type clients struct {
	m     sync.Mutex
	clock clock.Clock

	clientsTTL time.Duration
	clients    map[string]*client
}

// newClients creates a new clients object
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

// active checks whether a certain client is active
func (c *clients) active(pbClient *pbgo.Client) bool {
	client, ok := c.clients[pbClient.Id]
	if !ok {
		return false
	}
	return !client.expired(c.clock, c.clientsTTL)
}

// hasNewProducts checks whether the client's request contains products that
// were not present when the client was last seen. This detects the case where
// a tracer adds a new RC product (e.g., FFE_FLAGS) after the initial connection,
// which should trigger a cache bypass so the Agent fetches configs for the new product.
func (c *clients) hasNewProducts(pbClient *pbgo.Client) bool {
	c.m.Lock()
	defer c.m.Unlock()
	existing, ok := c.clients[pbClient.Id]
	if !ok {
		return false
	}
	for _, newProduct := range pbClient.Products {
		found := false
		for _, existingProduct := range existing.pbClient.Products {
			if newProduct == existingProduct {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
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
