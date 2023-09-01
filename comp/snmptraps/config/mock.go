// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/comp/netflow/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule provides the default config, and allows tests to override it by
// providing `fx.Replace(&TrapsConfig{...})`; a value replaced this way will
// have default values set sensibly if they aren't provided.
var MockModule = fxutil.Component(
	fx.Provide(func(tc *TrapsConfig) Component {
		return &configService{conf: tc}
	}),
	fx.Supply(&TrapsConfig{}),
	fx.Decorate(
		// Allow tests to inject incomplete config and have defaults set automatically
		func(conf *TrapsConfig, hnService hostname.Component) (*TrapsConfig, error) {
			return conf, conf.SetDefaults(hnService.Get(), "default")
		},
	),
)
