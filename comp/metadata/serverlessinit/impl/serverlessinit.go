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

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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
	log            log.Component
	eligible       bool
	// uuid is the payload uuid for the current identity, rotated when the
	// identity changes; a same-identity resume keeps it. Serverless containers
	// do not share a host GUID, so it starts fresh per publish.
	uuid           string
	lastResourceID string
}

// Requires defines the dependencies for the serverlessinit component
type Requires struct {
	Lc     compdef.Lifecycle
	Log    log.Component
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
		log:            deps.Log,
		eligible:       serverlessinit.InventoryEligible(deps.Config, deps.Provider),
	}

	if c.applyFields() {
		deps.Lc.Append(compdef.Hook{OnStart: func(context.Context) error {
			c.submitFirst()
			return nil
		}})
	}

	return Provides{Comp: c}
}

// applyFields installs the full field set for the provider's current identity,
// rotating the payload uuid when the identity changed — a same-identity resume
// keeps it. It reports whether the built field set passed validation: an
// invalid set is not applied, and the submission gate withholds any payload
// without a resource id. Field set and uuid land in one atomic batch, so a
// concurrent serialization never mixes a rotated uuid with the previous
// field set.
func (c *serverlessinitComp) applyFields() bool {
	if !c.eligible {
		return false
	}
	data := c.provider.GetInventoryData()
	fields := buildFields(data, c.params, c.conf)
	if reason := serverlessinit.ValidatePayload(fields); reason != nil {
		c.log.Debugf("serverless inventory payload not submitted: %v", reason)
		return false
	}

	if data.ResourceID != c.lastResourceID {
		c.uuid = uuid.New().String()
		c.lastResourceID = data.ResourceID
	}
	c.inventoryAgent.SetBatch(fields, c.uuid)
	return true
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
	if c.applyFields() {
		c.submitFirst()
	}
}
