// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"context"

	"go.uber.org/zap"

	"go.opentelemetry.io/collector/confmap"

	"github.com/DataDog/datadog-agent/comp/core/config"
	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/def"
)

type ddConverter struct {
	coreConfig config.Component
	logger     *zap.Logger
}

var (
	_ confmap.Converter = (*ddConverter)(nil)
)

// Requires defines the converter component required dependencies.
//
// An agent core configuration component dep is expected. A nil
// core config component will prevent enhancing the configuration
// with core agent config elements if any are missing from the provided
// OTel configuration. For example, when building in an environment that
// requires an argument-less constructor, such as with ocb. In this case,
// the core config component is not available and the converter will not
// attempt to enhance the configuration using agent data.
type Requires struct {
	Conf config.Component
}

// NewFactory returns a new converter factory.
func NewFactory() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverter)
}

func newConverter(set confmap.ConverterSettings) confmap.Converter {
	return &ddConverter{
		logger: set.Logger,
	}
}

// NewConverterForAgent currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConverterForAgent(reqs Requires) (converter.Component, error) {
	return &ddConverter{
		coreConfig: reqs.Conf,
	}, nil
}

// Convert autoconfigures conf and stores both the provided and enhanced conf.
func (c *ddConverter) Convert(_ context.Context, conf *confmap.Conf) error {
	c.enhanceConfig(conf)
	return nil
}
