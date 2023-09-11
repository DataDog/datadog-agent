// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func newMock(conf *NetflowConfig) Component {
	return &configService{conf}
}

// MockModule defines the fx options for the mock component.
// Injecting MockModule will provide default config;
// override this with fx.Replace(&config.NetflowConfig{...}).
// Note that defaults will always be populated.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
	fx.Supply(&NetflowConfig{}),
	fx.Decorate(
		// Allow tests to inject incomplete config and have defaults set automatically
		func(conf *NetflowConfig) (*NetflowConfig, error) {
			return conf, conf.SetDefaults("default")
		},
	),
)
