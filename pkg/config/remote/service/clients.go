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

type cacheBypassClients struct {
	clock    clock.Clock
	requests chan chan struct{}

	capacity int

	// Fixed window rate limiting
	// It allows client requests spikes while limiting the global amount of request
	mu struct {
		sync.Mutex
		currentWindow  time.Time
		windowDuration time.Duration
		allowance      int
	}
}

func newCacheBypassClients(
	clock clock.Clock,
	capacity int,
	windowDuration time.Duration,
) *cacheBypassClients {
	c := &cacheBypassClients{
		clock:    clock,
		requests: make(chan chan struct{}),
		capacity: capacity,
	}
	c.mu.currentWindow = clock.Now().UTC().Truncate(windowDuration)
	c.mu.windowDuration = windowDuration
	c.mu.allowance = capacity
	return c
}

func (c *cacheBypassClients) setWindowDuration(windowDuration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mu.windowDuration = windowDuration
}

func (c *cacheBypassClients) Limit() bool {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	window := now.Truncate(c.mu.windowDuration)

	// If we're in a new window, reset the allowance
	if c.mu.currentWindow != window {
		c.mu.currentWindow = window
		c.mu.allowance = c.capacity
	}

	if c.mu.allowance <= 0 {
		// If the window is already overflowed limit the request
		return true
	}
	c.mu.allowance--
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
