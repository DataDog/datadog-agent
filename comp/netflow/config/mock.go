// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package config

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func newMock(conf *NetflowConfig, logger log.Component) (Component, error) {
	if err := conf.SetDefaults("default", logger); err != nil {
		return nil, err
	}
	// TODO Currently reverse DNS enrichment is disabled by default for the agent but we want it enabled by default for tests.
	// Move this to conf.SetDefaults() if/when we enable it by default for the agent.
	conf.ReverseDNSEnrichmentEnabled = true
	return &configService{conf}, nil
}

// MockModule defines the fx options for the mock component.
// Injecting MockModule will provide default config;
// override this with fx.Replace(&config.NetflowConfig{...}).
// Defaults will always be populated.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Supply(&NetflowConfig{}))
}
