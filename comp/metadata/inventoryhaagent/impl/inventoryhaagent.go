// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package inventoryhaagentimpl implements the inventoryhaagentimpl component interface
package inventoryhaagentimpl

import (
	"context"
	"crypto/tls"
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	inventoryhaagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryhaagent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// Requires defines the dependencies for the inventoryhaagentimpl component
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	HaAgent    haagent.Component
}

// Provides defines the output of the inventoryhaagentimpl component
type Provides struct {
	Comp                 inventoryhaagent.Component
	Provider             runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
	Endpoint             api.AgentEndpointProvider
}

// NewComponent creates a new inventoryhaagentimpl component
func NewComponent(reqs Requires) (Provides, error) {
	hname, _ := hostname.Get(context.Background())
	// HTTP client need not verify ha-agent cert since it's self-signed
	// at start-up. TLS used for encryption not authentication.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	i := &inventoryhaagentimpl{
		conf:     reqs.Config,
		log:      reqs.Log,
		hostname: hname,
		data:     make(haAgentMetadata),
		haAgent:  reqs.HaAgent,
	}

	i.InventoryPayload = util.CreateInventoryPayload(reqs.Config, reqs.Log, reqs.Serializer, i.getPayload, "ha-agent.json")

	if i.Enabled {
		// We want to be notified when the configuration is updated
		reqs.Config.OnUpdate(func(_ string, _, _ any) { i.Refresh() })
	}

	return Provides{
		Comp:                 i,
		Provider:             i.MetadataProvider(),
		FlareProvider:        i.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(i),
		Endpoint:             api.NewAgentEndpointProvider(i.writePayloadAsJSON, "/metadata/inventory-ha-agent", "GET"),
	}, nil
}
