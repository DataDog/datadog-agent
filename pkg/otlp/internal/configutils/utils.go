// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package configutils

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configunmarshaler"
	"go.opentelemetry.io/collector/service"
	"gopkg.in/yaml.v2"
)

// NewMapFromYAMLString creates a config.Map from a YAML-formatted configuration string.
// Adapted from: https://github.com/open-telemetry/opentelemetry-collector/blob/v0.41.0/config/configmapprovider/inmemory.go
func NewMapFromYAMLString(cfg string) (*config.Map, error) {
	inp := strings.NewReader(cfg)
	content, err := ioutil.ReadAll(inp)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err = yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("unable to parse yaml: %w", err)
	}

	return config.NewMapFromStringMap(data), nil
}

const (
	mapSchemeName = "map"
	mapLocation   = "map:hardcoded"
)

var _ config.MapProvider = (*mapProvider)(nil)

type mapProvider struct {
	cfg *config.Map
}

func (m *mapProvider) Retrieve(_ context.Context, uri string, _ config.WatcherFunc) (config.Retrieved, error) {
	// We only support the constant location 'map:hardcoded'
	if uri != mapLocation {
		return config.Retrieved{}, fmt.Errorf("%v location is not supported by %v provider", uri, mapSchemeName)
	}

	return config.Retrieved{Map: m.cfg}, nil
}

func (m *mapProvider) Scheme() string {
	return mapSchemeName
}

func (m *mapProvider) Shutdown(context.Context) error {
	return nil
}

// NewConfigProviderFromMap creates a service.ConfigProvider with a single constant provider `map`, built from a given *config.Map.
func NewConfigProviderFromMap(cfg *config.Map) service.ConfigProvider {
	provider := &mapProvider{cfg}
	settings := service.ConfigProviderSettings{
		Locations: []string{mapLocation},
		MapProviders: map[string]config.MapProvider{
			"map": provider,
		},
		MapConverters: []config.MapConverterFunc{},
		Unmarshaler:   configunmarshaler.NewDefault(),
	}
	cp, err := service.NewConfigProvider(settings)
	if err != nil {
		panic(err)
	}
	return cp
}
