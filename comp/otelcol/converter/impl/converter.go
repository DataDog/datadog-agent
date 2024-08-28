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
)

type ddConverter struct{}

var (
	_ confmap.Converter = (*ddConverter)(nil)
)

// NewConverter currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConverter() (converter.Component, error) {
	return &ddConverter{}, nil
}

// Convert autoconfigures conf and stores both the provided and enhanced conf.
func (c *ddConverter) Convert(_ context.Context, conf *confmap.Conf) error {
	enhanceConfig(conf)
	return nil
}
