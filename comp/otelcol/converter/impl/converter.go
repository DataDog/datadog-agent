// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"context"

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
	provided string
	enhanced string
}

// NewConverter currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConverter() (converter.Component, error) {
	return &ddConverter{
		confDump: confDump{
			provided: "not supported",
			enhanced: "not supported",
		},
	}, nil
}

func (c *ddConverter) Convert(ctx context.Context, conf *confmap.Conf) error {
	// c.addProvidedConf(conf)

	enhanceConfig(conf)

	// c.addEnhancedConf(conf)
	return nil
}

func enhanceConfig(conf *confmap.Conf) {
	// extensions
	for _, component := range extensions {
		if ExtensionIsInServicePipeline(conf, component) {
			continue
		}
		addComponentToConfig(conf, component)
		addExtensionToPipeline(conf, component)
	}

	// infra attributes processor
	addProcessorToPipelinesWithDDExporter(conf, infraAttributesProcessor)

	// prometheus receiver
	addPrometheusReceiver(conf, prometheusReceiver)
}

// nolint: deadcode, unused
func (c *ddConverter) addProvidedConf(conf *confmap.Conf) error {
	bytesConf, err := confToString(conf)
	if err != nil {
		return err
	}

	c.confDump.provided = bytesConf
	return nil
}

// nolint: deadcode, unused
func (c *ddConverter) addEnhancedConf(conf *confmap.Conf) error {
	bytesConf, err := confToString(conf)
	if err != nil {
		return err
	}

	c.confDump.enhanced = bytesConf
	return nil
}

// GetProvidedConf returns a string representing the collector configuration passed
// by the user.
// Note: this is currently not supported.
func (c *ddConverter) GetProvidedConf() string {
	return c.confDump.provided
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
// Note: this is currently not supported.
func (c *ddConverter) GetEnhancedConf() string {
	return c.confDump.enhanced
}

// confToString takes in an *confmap.Conf and returns a string with the yaml
// representation. It takes advantage of the confmaps opaquevalue to redact any
// sensitive fields.
// Note: Currently not supported until the following upstream PR:
// https://github.com/open-telemetry/opentelemetry-collector/pull/10139 is merged.
// nolint: deadcode, unused
func confToString(conf *confmap.Conf) (string, error) {
	bytesConf, err := yaml.Marshal(conf.ToStringMap())
	if err != nil {
		return "", err
	}

	return string(bytesConf), nil
}
