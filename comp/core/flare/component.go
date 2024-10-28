// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements a component to generate flares from the agent.
//
// A flare is a archive containing all the information necessary to troubleshoot the Agent. When openeing a support
// ticket a flare might be requested. Flares contain the Agent logs, configurations and much more.
package flare

import (
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// Create creates a new flare locally and returns the path to the flare file.
	//
	// If providerTimeout is 0 or negative, the timeout from the configuration will be used.
	Create(pdata ProfileData, providerTimeout time.Duration, ipcError error) (string, error)
	// Send sends a flare archive to Datadog.
	Send(flarePath string, caseID string, email string, source helpers.FlareSource) (string, error)
}

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newFlare),
		fx.Supply(params))
}
