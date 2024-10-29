// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"errors"
	"sync"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v2"
)

type configStore struct {
	provided                *otelcol.Config
	enhanced                *otelcol.Config
	mu                      sync.RWMutex
	providedConfigSupported bool
	// TODO: Replace enhanced config with effective config once dependencies
	// are updated to v0.105.0 or newer (https://github.com/open-telemetry/opentelemetry-collector/pull/10139/files)
	effectiveConfig *confmap.Conf
}

// setProvidedConfigSupported sets the variable to determine if provided configuration read/write
// is supported with the current setup. Under OCB, we can only read enhanced configuration after
// any converters make changes to the config due to the implementation of ConfigWatcher interface.
func (c *configStore) setProvidedConfigSupported(supported bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.providedConfigSupported = supported
}

func (c *configStore) isProvidedConfigSupported() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.providedConfigSupported
}

// setProvidedConf stores the config into configStoreImpl.
func (c *configStore) setProvidedConf(config *otelcol.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.providedConfigSupported {
		return errProvidedConfigUnsupported
	}
	c.provided = config
	return nil
}

// setEnhancedConf stores the config into configStoreImpl.
func (c *configStore) setEnhancedConf(config *otelcol.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enhanced = config
}

func (c *configStore) setEffectiveConfig(conf *confmap.Conf) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.effectiveConfig = conf
}

func (c *configStore) getEffectiveConfigAsString() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return confMapToString(c.effectiveConfig)
}

func confMapToString(conf *confmap.Conf) (string, error) {
	bytesConf, err := yaml.Marshal(conf.ToStringMap())
	if err != nil {
		return "", err
	}

	return string(bytesConf), nil
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

var errProvidedConfigUnsupported = errors.New("reading/writing provided configuration is not supported")
