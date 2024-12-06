// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package configutils contains utilities used for reading configuration.
package configutils

import (
	"context"
	"fmt"
	"io"
	"strings"

	"go.opentelemetry.io/collector/confmap"
	"gopkg.in/yaml.v2"
)

// NewMapFromYAMLString creates a confmap.Conf from a YAML-formatted configuration string.
// Adapted from: https://github.com/open-telemetry/opentelemetry-collector/blob/v0.41.0/config/configmapprovider/inmemory.go
func NewMapFromYAMLString(cfg string) (*confmap.Conf, error) {
	inp := strings.NewReader(cfg)
	content, err := io.ReadAll(inp)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err = yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("unable to parse yaml: %w", err)
	}

	return confmap.NewFromStringMap(data), nil
}

const (
	mapSchemeName = "map"
	mapLocation   = "map:hardcoded"
)

var _ confmap.Provider = (*mapProvider)(nil)

type mapProvider struct {
	cfg *confmap.Conf
}

// providerFactory is implementation of confmap.ProviderFactory.
type providerFactory struct {
	cfg *confmap.Conf
}

// NewProviderFactory creates a new confmap.ProviderFactory.
func NewProviderFactory(cfg *confmap.Conf) confmap.ProviderFactory {
	return &providerFactory{cfg: cfg}
}

// Create creates a new confmap.Provider.
func (p *providerFactory) Create(_ confmap.ProviderSettings) confmap.Provider {
	return &mapProvider{cfg: p.cfg}
}

func (m *mapProvider) Retrieve(_ context.Context, uri string, _ confmap.WatcherFunc) (*confmap.Retrieved, error) {
	// We only support the constant location 'map:hardcoded'
	if uri != mapLocation {
		return &confmap.Retrieved{}, fmt.Errorf("%v location is not supported by %v provider", uri, mapSchemeName)
	}

	return confmap.NewRetrieved(m.cfg.ToStringMap())
}

func (m *mapProvider) Scheme() string {
	return mapSchemeName
}

func (m *mapProvider) Shutdown(context.Context) error {
	return nil
}
