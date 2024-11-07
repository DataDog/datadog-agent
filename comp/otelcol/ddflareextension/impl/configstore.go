// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"sync"

	"go.opentelemetry.io/collector/confmap"
	"gopkg.in/yaml.v2"
)

type configStore struct {
	provided *confmap.Conf
	enhanced *confmap.Conf
	mu       sync.RWMutex
}

// setProvidedConf stores the config into configStoreImpl.
func (c *configStore) setProvidedConf(config *confmap.Conf) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.provided = config
}

// setEnhancedConf stores the config into configStoreImpl.
func (c *configStore) setEnhancedConf(config *confmap.Conf) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enhanced = config
}

// getProvidedConf returns a string representing the enhanced collector configuration.
func (c *configStore) getProvidedConf() (*confmap.Conf, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.provided, nil
}

// getEnhancedConf returns a string representing the enhanced collector configuration.
func (c *configStore) getEnhancedConf() (*confmap.Conf, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.enhanced, nil
}

// getProvidedConfAsString returns a string representing the enhanced collector configuration string.
func (c *configStore) getProvidedConfAsString() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bytesConf, err := yaml.Marshal(c.provided.ToStringMap())
	if err != nil {
		return "", err
	}
	return string(bytesConf), nil
}

// getEnhancedConfAsString returns a string representing the enhanced collector configuration string.
func (c *configStore) getEnhancedConfAsString() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bytesConf, err := yaml.Marshal(c.enhanced.ToStringMap())
	if err != nil {
		return "", err
	}
	return string(bytesConf), nil
}
