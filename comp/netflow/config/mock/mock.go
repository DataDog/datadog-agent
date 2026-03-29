// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the netflow config component.
package mock

import (
	"go.uber.org/fx"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	config "github.com/DataDog/datadog-agent/comp/netflow/config/def"
	configimpl "github.com/DataDog/datadog-agent/comp/netflow/config/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockConfigService struct {
	conf *config.NetflowConfig
}

func (m *mockConfigService) Get() *config.NetflowConfig {
	return m.conf
}

func newMock(conf *config.NetflowConfig, logger log.Component) (configimpl.Provides, error) {
	if err := conf.SetDefaults("default", logger); err != nil {
		return configimpl.Provides{}, err
	}
	// TODO Currently reverse DNS enrichment is disabled by default for the agent but we want it enabled by default for tests.
	// Move this to conf.SetDefaults() if/when we enable it by default for the agent.
	conf.ReverseDNSEnrichmentEnabled = true
	return configimpl.Provides{Comp: &mockConfigService{conf}}, nil
}

// MockModule defines the fx options for the mock component.
// Injecting MockModule will provide default config;
// override this with fx.Replace(&config.NetflowConfig{...}).
// Defaults will always be populated.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Supply(&config.NetflowConfig{}))
}
