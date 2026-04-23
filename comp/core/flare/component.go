// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements a component to generate flares from the agent.
//
// A flare is a archive containing all the information necessary to troubleshoot the Agent. When opening a support
// ticket a flare might be requested. Flares contain the Agent logs, configurations and much more.
package flare

import (
	"go.uber.org/fx"

	flaredef "github.com/DataDog/datadog-agent/comp/core/flare/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-configuration

// Component is the component type.
type Component = flaredef.Component

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newFlare),
		fx.Supply(params))
}
