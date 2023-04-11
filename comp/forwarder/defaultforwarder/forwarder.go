// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	config config.Component
	Params Params
}

func newForwarder(dep dependencies) Component {
	if dep.Params.UseNoopForwarder {
		return NoopForwarder{}
	}
	return NewDefaultForwarder(dep.config, dep.Params.Options)
}

func newMockForwarder(config config.Component) Component {
	return NewDefaultForwarder(config, NewOptions(config, nil))
}
