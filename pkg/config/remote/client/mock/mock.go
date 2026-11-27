// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package mock

import (
	"maps"
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Client is a fake Remote Config client.
// It is safe for concurrent use.
// Callbacks are invoked without holding the internal lock, so they may call back into the fake.
type Client struct {
	mu        sync.Mutex
	callbacks map[string][]func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))
	configs   map[string]map[string]state.RawConfig
	applied   map[string]state.ApplyStatus
	proofs    map[string]state.ConfigTUFProof
}

// New returns a fake client without subscribers.
func New() *Client {
	return &Client{
		callbacks: make(map[string][]func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))),
		configs:   make(map[string]map[string]state.RawConfig),
		applied:   make(map[string]state.ApplyStatus),
		proofs:    make(map[string]state.ConfigTUFProof),
	}
}

// Subscribe registers cb for updates of the product.
func (c *Client) Subscribe(product string, cb func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks[product] = append(c.callbacks[product], cb)
}

// SubscribeIgnoreExpiration registers cb for updates of the product.
// The fake has no signature expiry, so it behaves like Subscribe.
func (c *Client) SubscribeIgnoreExpiration(product string, cb func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.Subscribe(product, cb)
}

// GetConfigs returns the configs last set by SetConfigs or delivered by Update for the product.
func (c *Client) GetConfigs(product string) map[string]state.RawConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.configs[product])
}

// UpdateApplyStatus records the apply status of a config path.
// Like the real client, it ignores paths that are not in the current configs of any product.
func (c *Client) UpdateApplyStatus(path string, status state.ApplyStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, configs := range c.configs {
		if _, ok := configs[path]; ok {
			c.applied[path] = status
			return
		}
	}
}

// GetConfigTUFProof returns the proof set by SetConfigTUFProof for the target path.
func (c *Client) GetConfigTUFProof(targetPath string) (state.ConfigTUFProof, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	proof, ok := c.proofs[targetPath]
	return proof, ok
}

// ---- Test helpers: the methods below do not exist on the real client. ----

// Subscribers returns the number of callbacks registered for the product.
func (c *Client) Subscribers(product string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.callbacks[product])
}

// ApplyStatuses returns a copy of every apply status recorded so far, by config path.
func (c *Client) ApplyStatuses() map[string]state.ApplyStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.applied)
}

// SetConfigs sets the configs returned by GetConfigs for the product.
func (c *Client) SetConfigs(product string, configs map[string]state.RawConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs[product] = maps.Clone(configs)
}

// Update makes configs the current configs of the product and delivers them to every subscriber,
// passing applyStateCallback as the apply status callback.
func (c *Client) Update(product string, configs map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	c.mu.Lock()
	c.configs[product] = maps.Clone(configs)
	callbacks := slices.Clone(c.callbacks[product])
	c.mu.Unlock()

	for _, cb := range callbacks {
		cb(maps.Clone(configs), applyStateCallback)
	}
}

// SetConfigTUFProof sets the proof returned by GetConfigTUFProof for its target path.
func (c *Client) SetConfigTUFProof(proof state.ConfigTUFProof) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.proofs[proof.TargetPath] = proof
}

// Component is a fake of the rcclient component.
// It adds the data.Product typed Subscribe and SubscribeAgentTask to Client.
type Component struct {
	*Client
}

// NewComponent returns a fake rcclient component without subscribers.
func NewComponent() *Component {
	return &Component{Client: New()}
}

// Subscribe registers cb for updates of the product.
func (c *Component) Subscribe(product data.Product, cb func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.Client.Subscribe(string(product), cb)
}

// SubscribeAgentTask does nothing: the fake has no AGENT_TASK product to deliver.
func (c *Component) SubscribeAgentTask() {}
