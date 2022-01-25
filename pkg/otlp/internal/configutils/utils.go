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
	"go.opentelemetry.io/collector/config/configmapprovider"
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

//NewProviderFromMap creates a configmapprovider.Provider that retrieves a given config.Map.
func NewProviderFromMap(cfg *config.Map) configmapprovider.Provider {
	return &mapProvider{cfg}
}

var _ configmapprovider.Provider = (*mapProvider)(nil)

type mapProvider struct {
	cfg *config.Map
}

func (m *mapProvider) Retrieve(context.Context, func(*configmapprovider.ChangeEvent)) (configmapprovider.Retrieved, error) {
	return configmapprovider.NewRetrieved(func(context.Context) (*config.Map, error) {
		return m.cfg, nil
	})
}

func (m *mapProvider) Shutdown(context.Context) error {
	return nil
}
