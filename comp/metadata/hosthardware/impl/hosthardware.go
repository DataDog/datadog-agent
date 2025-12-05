// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hosthardwareimpl implements a component to generate the 'host_hardware' metadata payload for inventory.
package hosthardwareimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	hosthardware "github.com/DataDog/datadog-agent/comp/metadata/hosthardware/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/inventory/hardware"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const flareFileName = "hosthardware.json"

type hostHardwareMetadata struct {
	Manufacturer      string `json:"manufacturer"`
	Model             string `json:"model"`
	SerialNumber      string `json:"serial_number"`
	EnclosureType     string `json:"enclosure_type"`
	EnclosureTypeName string `json:"enclosure_type_name"`
	HostType          string `json:"host_type"`
}

type hostHardware struct {
	util.InventoryPayload

	log      log.Component
	conf     config.Component
	hostname string
	data     *hostHardwareMetadata
}

type Payload struct {
	Hostname  string                `json:"hostname"`
	Timestamp int64                 `json:"timestamp"`
	Metadata  *hostHardwareMetadata `json:"host_hardware_metadata"`
	UUID      string                `json:"uuid"`
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
	IPCClient  ipc.HTTPClient
}

type Provides struct {
	Comp          hosthardware.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

func NewHardwareHostProvider(deps Requires) Provides {
	hname, _ := deps.Hostname.Get(context.Background())
	hh := &hostHardware{
		log:      deps.Log,
		conf:     deps.Config,
		hostname: hname,
		data:     &hostHardwareMetadata{},
	}
	hh.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, hh.getPayload, "hosthardware.json")

	// Override default intervals for hardware metadata
	// Hardware info changes infrequently, so check less often
	hh.InventoryPayload.MinInterval = 10 * time.Minute // Check every 10 minutes
	hh.InventoryPayload.MaxInterval = 1 * time.Hour    // Send every 1 hour

	// Only enable hardware metadata collection for end user device infrastructure mode
	infraMode := deps.Config.GetString("infrastructure_mode")
	isEndUserDevice := infraMode == "end_user_device"
	hh.InventoryPayload.Enabled = hh.InventoryPayload.Enabled && isEndUserDevice

	var provider runnerimpl.Provider
	if hh.InventoryPayload.Enabled {
		provider = hh.MetadataProvider()
		deps.Log.Info("Hardware metadata collection enabled for end user device mode")
	} else {
		deps.Log.Debugf("Hardware metadata collection disabled: infrastructure_mode is '%s' (requires 'end_user_device')", infraMode)
	}

	return Provides{
		Comp:          hh,
		Provider:      provider,
		FlareProvider: hh.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(hh.writePayloadAsJSON, "/metadata/host-hardware", "GET"),
	}
}

func (hh *hostHardware) fillData() {
	hardwareInfo, err := hardware.Collect()
	if err != nil {
		hh.log.Errorf("Failed to collect hardware information: %v", err)
		return
	}
	hh.data.Manufacturer = hardwareInfo.Manufacturer
	hh.data.Model = hardwareInfo.Model
	hh.data.SerialNumber = hardwareInfo.SerialNumber
	hh.data.EnclosureType = hardwareInfo.EnclosureType
	hh.data.EnclosureTypeName = hardwareInfo.EnclosureTypeName
	hh.data.HostType = hardwareInfo.HostType
}

func (hh *hostHardware) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON calls getPayload which already scrub the data
	scrubbed, err := hh.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func (hh *hostHardware) getPayload() marshaler.JSONMarshaler {

	hh.fillData()
	if hh.data == nil {
		return nil
	}

	return &Payload{
		Hostname:  hh.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  hh.data,
		UUID:      uuid.GetUUID(),
	}
}
