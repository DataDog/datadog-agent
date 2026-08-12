// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package workloadbalancingimpl implements the workloadbalancing metadata component interface
package workloadbalancingimpl

import (
	"context"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	runnerdef "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	workloadbalancing "github.com/DataDog/datadog-agent/comp/metadata/workloadbalancing/def"
	workloadbalancingcomp "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Requires defines the dependencies for the workloadbalancingimpl component
type Requires struct {
	Log               log.Component
	Config            config.Component
	Serializer        serializer.MetricSerializer
	WorkloadBalancing workloadbalancingcomp.Component
	Hostname          hostnameinterface.Component
}

// Provides defines the output of the workloadbalancingimpl component
type Provides struct {
	Comp                 workloadbalancing.Component
	Provider             runnerdef.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
	Endpoint             api.AgentEndpointProvider
}

// NewComponent creates a new workloadbalancingimpl component
func NewComponent(reqs Requires) (Provides, error) {
	hname, _ := reqs.Hostname.Get(context.Background())
	i := &workloadbalancingimpl{
		conf:              reqs.Config,
		log:               reqs.Log,
		hostname:          hname,
		data:              &workloadBalancingMetadata{},
		workloadBalancing: reqs.WorkloadBalancing,
	}

	if !i.workloadBalancing.Enabled() {
		i.log.Debugf("Workload balancing metadata unavailable as Agent workload balancing is disabled")
	}

	i.InventoryPayload = util.CreateInventoryPayload(reqs.Config, reqs.Log, reqs.Serializer, i.getPayload, "workload-balancing.json")

	return Provides{
		Comp:                 i,
		Provider:             i.MetadataProvider(),
		FlareProvider:        i.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(i),
		Endpoint:             api.NewAgentEndpointProvider(i.writePayloadAsJSON, "/metadata/workload-balancing", "GET"),
	}, nil
}
