// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"sync"
)

type configStore struct {
	providedConf string
	envConf      string
	enhancedConf string
	mu           sync.RWMutex
}

func (c *configStore) setProvided(conf string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.providedConf = conf
}

func (c *configStore) set(envConf string, enhancedConf string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enhancedConf = enhancedConf
	c.envConf = envConf
}

func (c *configStore) getProvidedConf() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.providedConf
}

func (c *configStore) getEnhancedConf() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.enhancedConf
}

func (c *configStore) getEnvConf() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.envConf
}
