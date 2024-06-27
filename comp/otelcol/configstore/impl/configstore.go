// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configstoreimpl provides the implementation of the otel-agent converter.
package configstoreimpl

import (
	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/def"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v2"
)

type configStoreImpl struct {
	provided *otelcol.Config
	enhanced *otelcol.Config
}

// NewConfigStore currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConfigStore() (configstore.Component, error) {
	return &configStoreImpl{}, nil
}

// AddProvidedConf stores the config into configStoreImpl.
func (c *configStoreImpl) AddProvidedConf(config *otelcol.Config) {
	c.provided = config
}

// AddEnhancedConf stores the config into configStoreImpl.
func (c *configStoreImpl) AddEnhancedConf(config *otelcol.Config) {
	c.enhanced = config
}

// GetProvidedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetProvidedConf() (*confmap.Conf, error) {
	conf := confmap.New()
	conf.Marshal(c.provided)
	return conf, nil
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetEnhancedConf() (*confmap.Conf, error) {
	conf := confmap.New()
	conf.Marshal(c.enhanced)
	return conf, nil
}

// GetProvidedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetProvidedConfAsString() (string, error) {
	confstr, err := confToString(c.provided)

	return confstr, err
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
func (c *configStoreImpl) GetEnhancedConfAsString() (string, error) {
	confstr, err := confToString(c.enhanced)

	return confstr, err
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
