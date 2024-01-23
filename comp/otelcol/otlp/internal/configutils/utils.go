// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package configutils contains utilities used for reading configuration.
package configutils

import (
	"context"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
)

// NewMapFromYAMLString creates a confmap.Conf from a YAML-formatted configuration string.
// Adapted from: https://github.com/open-telemetry/opentelemetry-collector/blob/v0.41.0/config/configmapprovider/inmemory.go
func NewMapFromYAMLString(cfg string) (*confmap.Conf, error) {
	panic("not called")
}

const (
	mapSchemeName = "map"
	mapLocation   = "map:hardcoded"
)

var _ confmap.Provider = (*mapProvider)(nil)

type mapProvider struct {
	cfg *confmap.Conf
}

func (m *mapProvider) Retrieve(_ context.Context, uri string, _ confmap.WatcherFunc) (*confmap.Retrieved, error) {
	panic("not called")
}

func (m *mapProvider) Scheme() string {
	panic("not called")
}

func (m *mapProvider) Shutdown(context.Context) error {
	panic("not called")
}

// NewConfigProviderFromMap creates a service.ConfigProvider with a single constant provider `map`, built from a given *confmap.Conf.
func NewConfigProviderFromMap(cfg *confmap.Conf) otelcol.ConfigProvider {
	panic("not called")
}
