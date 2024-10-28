// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package configimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	trapsconf "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	HostnameService hostname.Component
	Conf            *trapsconf.TrapsConfig
}

func newMockConfig(dep dependencies) (trapsconf.Component, error) {
	host, err := dep.HostnameService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	tc := dep.Conf
	if err := tc.SetDefaults(host, "default"); err != nil {
		return nil, err
	}
	return &configService{conf: tc}, nil
}

// MockModule provides the default config, and allows tests to override it by
// providing `fx.Replace(&TrapsConfig{...})`; a value replaced this way will
// have default values set sensibly if they aren't provided.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockConfig),
		fx.Supply(&trapsconf.TrapsConfig{Enabled: true}),
	)
}
