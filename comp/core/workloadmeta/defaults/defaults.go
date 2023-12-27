// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaults

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// DefaultParams creates a Params struct with the default configuration, setting the agent type
// to NodeAgent or ClusterAgent depending on the flavor and initializing the workloadmeta component
func DefaultParams() workloadmeta.Params {
	params := workloadmeta.Params{
		AgentType:  workloadmeta.NodeAgent,
		InitHelper: common.GetWorkloadmetaInit(),
	}
	if flavor.GetFlavor() == flavor.ClusterAgent {
		params.AgentType = workloadmeta.ClusterAgent
	}
	return params
}
