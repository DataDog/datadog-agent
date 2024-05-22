// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"go.uber.org/fx"

	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// Component is the agent component type.
type Component interface {
	// GetAgent returns a trace agent.
	GetAgent() *pkgagent.Agent
}

// Module defines the fx options for the agent component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAgent))
}
