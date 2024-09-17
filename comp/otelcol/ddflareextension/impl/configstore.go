// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"sync"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v2"
)

type configStore struct {
	provided *otelcol.Config
	enhanced *otelcol.Config
	mu       sync.RWMutex
}

// addProvidedConf stores the config into configStoreImpl.
func (c *configStore) addProvidedConf(config *otelcol.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.provided = config
}

// addEnhancedConf stores the config into configStoreImpl.
func (c *configStore) addEnhancedConf(config *otelcol.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enhanced = config
}

func confToString(conf *otelcol.Config) (string, error) {
	cfg := confmap.New()
	err := cfg.Marshal(conf)
	if err != nil {
		return "", err
	}
	bytesConf, err := yaml.Marshal(cfg.ToStringMap())
	if err != nil {
		return "", err
	}

	return string(bytesConf), nil
}

// getProvidedConf returns a string representing the enhanced collector configuration.
func (c *configStore) getProvidedConf() (*confmap.Conf, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conf := confmap.New()
	err := conf.Marshal(c.provided)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// getEnhancedConf returns a string representing the enhanced collector configuration.
func (c *configStore) getEnhancedConf() (*confmap.Conf, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conf := confmap.New()
	err := conf.Marshal(c.enhanced)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// getProvidedConfAsString returns a string representing the enhanced collector configuration string.
func (c *configStore) getProvidedConfAsString() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return confToString(c.provided)
}

// getEnhancedConfAsString returns a string representing the enhanced collector configuration string.
func (c *configStore) getEnhancedConfAsString() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return confToString(c.enhanced)
}
