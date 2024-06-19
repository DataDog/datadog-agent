// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"context"
	"fmt"

	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/def"
	"go.opentelemetry.io/collector/confmap"
	"gopkg.in/yaml.v3"
)

type ddConverter struct {
	confDump confDump
}

var (
	_ confmap.Converter = (*ddConverter)(nil)
)

type confDump struct {
	provided *confmap.Conf
	enhanced *confmap.Conf
}

// NewConverter currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConverter() (converter.Component, error) {
	return &ddConverter{
		confDump: confDump{},
	}, nil
}

// Convert autoconfigures conf and stores both the provided and enhanced conf.
func (c *ddConverter) Convert(_ context.Context, conf *confmap.Conf) error {
	// c.addProvidedConf(conf)

	enhanceConfig(conf)

	// c.addEnhancedConf(conf)
	return nil
}

func (c *ddConverter) addProvidedConf(conf *confmap.Conf) {
	c.confDump.provided = conf
}

// nolint: deadcode, unused
func (c *ddConverter) addEnhancedConf(conf *confmap.Conf) {
	c.confDump.enhanced = conf
}

// GetProvidedConf returns a confMap.Conf representing the collector configuration passed
// by the user.
func (c *ddConverter) GetProvidedConf() *confmap.Conf {
	return c.confDump.provided
}

// GetEnhancedConf returns a confMap.Conf representing the enhanced collector configuration.
// Note: this is currently not supported.
func (c *ddConverter) GetEnhancedConf() *confmap.Conf {
	return c.confDump.enhanced
}

// GetProvidedConfAsString returns a string representing the collector configuration passed
// by the user.
func (c *ddConverter) GetProvidedConfAsString() (string, error) {
	confstr, err := confToString(c.confDump.provided)

	return confstr, err
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
// Note: this is currently not supported.
func (c *ddConverter) GetEnhancedConfAsString() (string, error) {
	confstr, _ := confToString(c.confDump.enhanced)

	return confstr, fmt.Errorf("unsupported")
}

// confToString takes in an *confmap.Conf and returns a string with the yaml
// representation. It takes advantage of the confmaps opaquevalue to redact any
// sensitive fields.
// Note: Currently not supported until the following upstream PR:
// https://github.com/open-telemetry/opentelemetry-collector/pull/10139 is merged.
// nolint: deadcode, unused
func confToString(conf *confmap.Conf) (string, error) {
	if conf == nil {
		return "", fmt.Errorf("confmap provided was nil")
	}
	bytesConf, err := yaml.Marshal(conf.ToStringMap())
	if err != nil {
		return "", err
	}

	return string(bytesConf), nil
}
