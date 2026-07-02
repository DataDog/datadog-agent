// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/params"
)

// Params is the canonical, introspectable parameter set for the agent-health scenario.
type Params struct {
	OS   string `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12|amazon-linux-2023"`
	Arch string `scenario:"name=arch,default=x86_64,help=CPU architecture,enum=x86_64|arm64"`

	// Agent holds agent-installation parameters.
	Agent params.AgentParams
}

// NewParams returns fully-defaulted Params. This is the blessed constructor:
// it yields the same values the CLI produces for empty input.
func NewParams() *Params { return scenario.NewParams[Params]() }
