// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package config

import (
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func newMock(conf *NetflowConfig, logger log.Component) (Component, error) {
	if err := conf.SetDefaults("default", logger); err != nil {
		return nil, err
	}
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
