// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containercheck

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process/types"
)

type check struct {
}

func (c *check) IsEnabled() bool {
	return true
}

func (c *check) Run() (*types.Payload, error) {
	return &types.Payload{}, nil
}

func (c *check) Name() string {
	return "container"
}

type dependencies struct {
	fx.In

	coreConfig     config.Component
	sysProbeConfig sysprobeconfig.Component
}

func newCheck(deps dependencies) types.ProvidesCheck {
	return types.ProvidesCheck{
		Check: &check{},
	}
}
