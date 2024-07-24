// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configstoreimpl provides the implementation of the otel-agent configstore.
package configstoreimpl

import (
	"context"
	"fmt"
	"sync"

	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/def"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v2"
)

type configStoreImpl struct {
	provided *otelcol.Config
	enhanced *otelcol.Config
	mu       sync.RWMutex
}

// NewConfigStore currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConfigStore() (configstore.Component, error) {
	return &configStoreImpl{}, nil
}

func (c *configStoreImpl) AddConfigs(providedCPS otelcol.ConfigProviderSettings, enhancedCPS otelcol.ConfigProviderSettings, factories otelcol.Factories) error {
	// Provided
	ocpProvided, err := otelcol.NewConfigProvider(providedCPS)
	if err != nil {
		return fmt.Errorf("failed to create configprovider: %w", err)
	}
	providedConf, err := ocpProvided.Get(context.Background(), factories)
	if err != nil {
		return err
	}
	c.addProvidedConf(providedConf)

	// Enhanced
	ocpEnhanced, err := otelcol.NewConfigProvider(enhancedCPS)
	if err != nil {
		return fmt.Errorf("failed to create configprovider: %w", err)
	}
	enhancedConf, err := ocpEnhanced.Get(context.Background(), factories)
	if err != nil {
		return err
	}
	c.addEnhancedConf(enhancedConf)

	return nil
}

// addProvidedConf stores the config into configStoreImpl.
func (c *configStoreImpl) addProvidedConf(config *otelcol.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.provided = config
}

// addEnhancedConf stores the config into configStoreImpl.
func (c *configStoreImpl) addEnhancedConf(config *otelcol.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enhanced = config
}

// GetProvidedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetProvidedConf() (*confmap.Conf, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conf := confmap.New()
	err := conf.Marshal(c.provided)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetEnhancedConf() (*confmap.Conf, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conf := confmap.New()
	err := conf.Marshal(c.enhanced)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// GetProvidedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetProvidedConfAsString() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return confToString(c.provided)
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetEnhancedConfAsString() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return confToString(c.enhanced)
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
