// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package serverlessinitimpl implements the serverless-init inventory payload
// component: it owns the serverless field set of the inventoryagent payload
// and the first-payload submission.
package serverlessinitimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	iadef "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	serverlessinit "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
)

type serverlessinitComp struct {
	inventoryAgent iadef.Component
	provider       serverlessinit.InventoryProvider
	params         serverlessinit.Params
	conf           config.Component
	eligible       bool
}

// Requires defines the dependencies for the serverlessinit component
type Requires struct {
	Lc     compdef.Lifecycle
	Config config.Component
	// Params carry the process-level context the embedding binary resolves
	// before Fx starts.
	Params serverlessinit.Params
	// InventoryAgent is the payload this component drives: it applies the
	// serverless field set through SetBatch and submits through it.
	InventoryAgent iadef.Component
	// Provider is the platform contract; the embedding binary supplies its
	// cloud-service implementation.
	Provider serverlessinit.InventoryProvider
	// Runner runs the periodic submissions of the inventoryagent payload.
	Runner runnerdef.Component
}

// Provides defines the output of the serverlessinit component
type Provides struct {
	Comp serverlessinit.Component
}

// NewComponent creates a new serverlessinit component. Platforms whose
// resource identity is resolved before Fx construction publish at startup;
// the rest stay closed until their identity event calls
// InventoryIdentityArrived.
func NewComponent(deps Requires) Provides {
	c := &serverlessinitComp{
		inventoryAgent: deps.InventoryAgent,
		provider:       deps.Provider,
		params:         deps.Params,
		conf:           deps.Config,
		eligible:       serverlessinit.InventoryEligible(deps.Config, deps.Provider),
	}

	if c.eligible && deps.Provider.GetInventoryData().ResourceID != "" {
		c.applyFields()
		deps.Lc.Append(compdef.Hook{OnStart: func(context.Context) error {
			c.submitFirst()
			return nil
		}})
	}

	return Provides{Comp: c}
}

// applyFields applies the full serverless field set, opening the payload.
func (c *serverlessinitComp) applyFields() {
	c.inventoryAgent.SetBatch(buildFields(c.provider.GetInventoryData(), c.params, c.conf))
}

// submitFirst enqueues the startup payload and switches later runner
// submissions to the periodic report reason.
func (c *serverlessinitComp) submitFirst() {
	c.inventoryAgent.Submit()
	c.inventoryAgent.Set("report_reason", reportReasonPeriodic)
}

// InventoryIdentityArrived publishes the payload after the platform learned
// its resource identity at runtime.
func (c *serverlessinitComp) InventoryIdentityArrived() {
	if !c.eligible || c.provider.GetInventoryData().ResourceID == "" {
		return
	}
	c.applyFields()
	c.submitFirst()
}
