// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	helpers "github.com/DataDog/datadog-agent/comp/workloadbalancing/helpers"
)

type workloadBalancingConfigs struct {
	enabled bool
}

func newWorkloadBalancingConfigs(agentConfig config.Component) *workloadBalancingConfigs {
	return &workloadBalancingConfigs{
		enabled: helpers.IsEnabled(agentConfig),
	}
}
