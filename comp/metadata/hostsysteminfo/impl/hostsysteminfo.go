// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostsysteminfoimpl implements a component to generate the 'host_system_info' metadata payload for inventory.
package hostsysteminfoimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	hostsysteminfo "github.com/DataDog/datadog-agent/comp/metadata/hostsysteminfo/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const flareFileName = "hostsysteminfo.json"

type hostSystemInfoMetadata struct {
	Manufacturer string `json:"manufacturer"`
	ModelNumber  string `json:"model_number"`
	SerialNumber string `json:"serial_number"`
	ModelName    string `json:"model_name"`
	ChassisType  string `json:"chassis_type"`
	Identifier   string `json:"identifier"`
}

type hostSystemInfo struct {
	util.InventoryPayload

	log      log.Component
	conf     config.Component
	hostname string
	data     *hostSystemInfoMetadata
}

type Payload struct {
	Hostname  string                  `json:"hostname"`
	Timestamp int64                   `json:"timestamp"`
	Metadata  *hostSystemInfoMetadata `json:"host_system_info_metadata"`
	UUID      string                  `json:"uuid"`
}

func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

type Provides struct {
	Comp          hostsysteminfo.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

func NewSystemInfoProvider(deps Requires) Provides {
	hname, _ := deps.Hostname.Get(context.Background())
	hh := &hostSystemInfo{
		log:      deps.Log,
		conf:     deps.Config,
		hostname: hname,
		data:     &hostSystemInfoMetadata{},
	}
	hh.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, hh.getPayload, flareFileName)

	// Override default intervals for system info metadata
	// System info changes infrequently, so check less often
	hh.InventoryPayload.MinInterval = 1 * time.Hour
	hh.InventoryPayload.MaxInterval = 1 * time.Hour

	// Only enable system info metadata collection for end user device infrastructure mode on Windows and Darwin
	infraMode := deps.Config.GetString("infrastructure_mode")
	isEndUserDevice := infraMode == "end_user_device"
	isSupportedOS := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	hh.InventoryPayload.Enabled = hh.InventoryPayload.Enabled && isEndUserDevice && isSupportedOS

	var provider runnerimpl.Provider
	if hh.InventoryPayload.Enabled {
		provider = hh.MetadataProvider()
		deps.Log.Info("System info metadata collection enabled for end user device mode")
	} else {
		if !isSupportedOS {
			deps.Log.Debugf("System info metadata collection disabled: only supported on Windows and macOS (current OS: %s)", runtime.GOOS)
		} else {
			deps.Log.Debugf("System info metadata collection disabled: infrastructure_mode is '%s' (requires 'end_user_device')", infraMode)
		}
	}

	return Provides{
		Comp:          hh,
		Provider:      provider,
		FlareProvider: hh.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(hh.writePayloadAsJSON, "/metadata/host-system-info", "GET"),
	}
}

func (hh *hostSystemInfo) fillData() error {
	// System info collection is only supported on Windows and Darwin
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		hh.log.Debugf("System information collection not supported on %s", runtime.GOOS)
		hh.data = nil
		return nil
	}

	sysInfo, err := systeminfo.Collect()
	if err != nil {
		hh.log.Errorf("Failed to collect system information: %v", err)
		hh.data = &hostSystemInfoMetadata{}
		return err
	}

	// Handle case where collection returns nil data
	if sysInfo == nil {
		hh.log.Debug("System information collection returned no data")
		hh.data = &hostSystemInfoMetadata{}
		return nil
	}

	hh.data.Manufacturer = sysInfo.Manufacturer
	hh.data.ModelNumber = sysInfo.ModelNumber
	hh.data.SerialNumber = sysInfo.SerialNumber
	hh.data.ModelName = sysInfo.ModelName
	hh.data.ChassisType = sysInfo.ChassisType
	hh.data.Identifier = sysInfo.Identifier

	return nil
}

func (hh *hostSystemInfo) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON calls getPayload which already scrub the data
	scrubbed, err := hh.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func (hh *hostSystemInfo) getPayload() marshaler.JSONMarshaler {
	// Try to collect system info data
	if err := hh.fillData(); err != nil {
		hh.log.Debugf("Skipping system info metadata payload due to collection failure: %v", err)
		return nil
	}

	return &Payload{
		Hostname:  hh.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  hh.data,
		UUID:      uuid.GetUUID(),
	}
}
