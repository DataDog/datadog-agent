// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package agentprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"go.opentelemetry.io/collector/confmap"
)

const (
	schemeName = "dd"
)

// CollectorParams provides access to collector configuration parameters.
type CollectorParams interface {
	GetGoRuntimeMetrics() bool
}

type agentProvider struct {
	config configManager
	params CollectorParams
}

func NewFactory(agentConfig config.Component, params CollectorParams) confmap.ProviderFactory {
	return confmap.NewProviderFactory(newProvider(agentConfig, params))
}

func newProvider(agentConfig config.Component, params CollectorParams) confmap.CreateProviderFunc {
	return func(_ confmap.ProviderSettings) confmap.Provider {
		return &agentProvider{
			config: newConfigManager(agentConfig),
			params: params,
		}
	}
}

func (ap *agentProvider) Retrieve(_ context.Context, uri string, _ confmap.WatcherFunc) (*confmap.Retrieved, error) {
	if uri != "dd:" {
		return nil, fmt.Errorf("%q uri is not supported by %q provider", uri, schemeName)
	}
	if ap.config.config == nil {
		return nil, errors.New("agent config is not available")
	}

	if len(ap.config.endpoints) == 0 {
		return nil, errors.New("no valid endpoints configured: ensure Datadog agent configuration has 'api_key' and either 'apm_config.profiling_dd_url' or 'site' set")
	}

	stringMap := buildConfig(ap.config, ap.params)

	return confmap.NewRetrieved(stringMap)
}

func (ap *agentProvider) Scheme() string {
	return schemeName
}

func (*agentProvider) Shutdown(context.Context) error {
	return nil
}
