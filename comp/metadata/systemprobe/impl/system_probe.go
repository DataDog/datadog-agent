// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package systemprobeimpl implements the systemprobe metadata providers interface
package systemprobeimpl

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
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	systemprobemetadata "github.com/DataDog/datadog-agent/comp/metadata/systemprobe/def"
	configFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher/sysprobe"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	// for testing
	fetchSystemProbeConfig         = configFetcher.SystemProbeConfig
	fetchSystemProbeConfigBySource = configFetcher.SystemProbeConfigBySource
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string                 `json:"hostname"`
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"system_probe_metadata"`
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
	return nil, fmt.Errorf("could not split system-probe process payload any more, payload is too big for intake")
}

type systemprobe struct {
	util.InventoryPayload

	log          log.Component
	conf         config.Component
	sysprobeConf option.Option[sysprobeconfig.Component]
	hostname     string
}

// Requires defines the dependencies for the systemprobe metadata component
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	// We need the authtoken to be created so we requires the comp. It will be used by configFetcher.
	AuthToken      authtoken.Component
	SysProbeConfig option.Option[sysprobeconfig.Component]
}

// Provides defines the output of the systemprobe metadatacomponent
type Provides struct {
	Comp             systemprobemetadata.Component
	MetadataProvider runnerimpl.Provider
	FlareProvider    flaretypes.Provider
	Endpoint         api.AgentEndpointProvider
}

// NewComponent creates a new systemprobe metadata Component
func NewComponent(deps Requires) Provides {
	hname, _ := hostname.Get(context.Background())
	sb := &systemprobe{
		log:          deps.Log,
		conf:         deps.Config,
		hostname:     hname,
		sysprobeConf: deps.SysProbeConfig,
	}
	sb.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, sb.getPayload, "system-probe.json")

	return Provides{
		Comp:             sb,
		MetadataProvider: sb.MetadataProvider(),
		FlareProvider:    sb.FlareProvider(),
		Endpoint:         api.NewAgentEndpointProvider(sb.writePayloadAsJSON, "/metadata/system-probe", "GET"),
	}
}

func (sb *systemprobe) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON calls getPayload which already scrub the data
	scrubbed, err := sb.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func (sb *systemprobe) getConfigLayers() map[string]interface{} {
	metadata := map[string]interface{}{
		"agent_version": version.AgentVersion,
	}

	if !sb.conf.GetBool("inventories_configuration_enabled") {
		return metadata
	}

	sysprobeConf, isSet := sb.sysprobeConf.Get()
	if !isSet {
		sb.log.Debugf("system-probe config not available: disabling systemprobe metadata")
		return metadata
	}

	rawLayers, err := fetchSystemProbeConfigBySource(sysprobeConf)
	if err != nil {
		sb.log.Debugf("error fetching system-probe config layers: %s", err)
		return metadata
	}

	configBySources := map[model.Source]interface{}{}
	if err := json.Unmarshal([]byte(rawLayers), &configBySources); err != nil {
		sb.log.Debugf("error unmarshalling systemprobe config by source: %s", err)
		return metadata
	}

	configutils.InjectConfigLayersForMetadata(sb.conf, configBySources, metadata)
	return metadata
}

func (sb *systemprobe) getPayload() marshaler.JSONMarshaler {
	return &Payload{
		Hostname:  sb.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  sb.getConfigLayers(),
	}
}
