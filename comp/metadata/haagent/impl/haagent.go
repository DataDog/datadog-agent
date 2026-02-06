// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package haagentimpl implements the haagentimpl component interface
package haagentimpl

import (
	"context"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	haagentcomp "github.com/DataDog/datadog-agent/comp/haagent/def"
	haagent "github.com/DataDog/datadog-agent/comp/metadata/haagent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Requires defines the dependencies for the haagentimpl component
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	HaAgent    haagentcomp.Component
	Hostname   hostnameinterface.Component
}

// Provides defines the output of the haagentimpl component
type Provides struct {
	Comp                 haagent.Component
	Provider             runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
	Endpoint             api.AgentEndpointProvider
}

// NewComponent creates a new haagentimpl component
func NewComponent(reqs Requires) (Provides, error) {
	hname, _ := reqs.Hostname.Get(context.Background())
	i := &haagentimpl{
		conf:     reqs.Config,
		log:      reqs.Log,
		hostname: hname,
		data:     &haAgentMetadata{},
		haAgent:  reqs.HaAgent,
	}

	if !i.haAgent.Enabled() {
		i.log.Debugf("HA Agent Metadata unavailable as HA Agent is disabled")
	}

	i.InventoryPayload = util.CreateInventoryPayload(reqs.Config, reqs.Log, reqs.Serializer, i.getPayload, "ha-agent.json")

	return Provides{
		Comp:                 i,
		Provider:             i.MetadataProvider(),
		FlareProvider:        i.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(i),
		Endpoint:             api.NewAgentEndpointProvider(i.writePayloadAsJSON, "/metadata/ha-agent", "GET"),
	}, nil
}
