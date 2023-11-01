// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package configimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	trapsconf "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func newMockConfig(tc *trapsconf.TrapsConfig, hnService hostname.Component) (trapsconf.Component, error) {
	host, err := hnService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	if err := tc.SetDefaults(host, "default"); err != nil {
		return nil, err
	}
	return &configService{conf: tc}, nil
}

// MockModule provides the default config, and allows tests to override it by
// providing `fx.Replace(&TrapsConfig{...})`; a value replaced this way will
// have default values set sensibly if they aren't provided.
var MockModule = fxutil.Component(
	fx.Provide(newMockConfig),
	fx.Supply(&trapsconf.TrapsConfig{}),
)
