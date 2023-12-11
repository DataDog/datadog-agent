// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	Config config.Component
	Log    log.Component
	Params Params
}

func newForwarder(dep dependencies) Component {
	return NewForwarder(dep.Config, dep.Log, dep.Params)
}

//nolint:revive // TODO(ASC) Fix revive linter
func NewForwarder(config config.Component, log log.Component, params Params) Component {
	if params.UseNoopForwarder {
		return NoopForwarder{}
	}
	return NewDefaultForwarder(config, log, params.Options)
}

func newMockForwarder(config config.Component, log log.Component) Component {
	return NewDefaultForwarder(config, log, NewOptions(config, log, nil))
}
