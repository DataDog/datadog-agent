// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package securityagentimpl implements the securityagent metadata providers interface
package securityagentimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	securityagent "github.com/DataDog/datadog-agent/comp/metadata/securityagent/def"
	configFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	// for testing
	fetchSecurityAgentConfig         = configFetcher.SecurityAgentConfig
	fetchSecurityAgentConfigBySource = configFetcher.SecurityAgentConfigBySource
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string                 `json:"hostname"`
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"security_agent_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split security-agent process payload any more, payload is too big for intake")
}

type secagent struct {
	util.InventoryPayload

	log      log.Component
	conf     config.Component
	hostname string
}

// Requires defines the dependencies for the securityagent metadata component
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	// We need the authtoken to be created so we requires the comp. It will be used by configFetcher.
	AuthToken authtoken.Component
}

// Provides defines the output of the securityagent metadata component
type Provides struct {
	Comp             securityagent.Component
	MetadataProvider runnerimpl.Provider
	FlareProvider    flaretypes.Provider
	Endpoint         api.AgentEndpointProvider
}

// NewComponent creates a new securityagent metadata Component
func NewComponent(deps Requires) Provides {
	hname, _ := hostname.Get(context.Background())
	sa := &secagent{
		log:      deps.Log,
		conf:     deps.Config,
		hostname: hname,
	}
	sa.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, sa.getPayload, "security-agent.json")

	return Provides{
		Comp:             sa,
		MetadataProvider: sa.MetadataProvider(),
		FlareProvider:    sa.FlareProvider(),
		Endpoint:         api.NewAgentEndpointProvider(sa.writePayloadAsJSON, "/metadata/security-agent", "GET"),
	}
}

func (sa *secagent) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON calls getPayload which already scrub the data
	scrubbed, err := sa.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func (sa *secagent) getConfigLayers() map[string]interface{} {
	metadata := map[string]interface{}{
		"agent_version": version.AgentVersion,
	}

	if !sa.conf.GetBool("inventories_configuration_enabled") {
		return metadata
	}

	rawLayers, err := fetchSecurityAgentConfigBySource(sa.conf)
	if err != nil {
		sa.log.Debugf("error fetching security-agent config layers: %s", err)
		return metadata
	}

	configBySources := map[model.Source]interface{}{}
	if err := json.Unmarshal([]byte(rawLayers), &configBySources); err != nil {
		sa.log.Debugf("error unmarshalling securityagent config by source: %s", err)
		return metadata
	}

	configutils.InjectConfigLayersForMetadata(sa.conf, configBySources, metadata)
	return metadata
}

func (sa *secagent) getPayload() marshaler.JSONMarshaler {
	return &Payload{
		Hostname:  sa.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  sa.getConfigLayers(),
	}
}
