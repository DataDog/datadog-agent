// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostgpuimpl implements a component to generate the 'host_gpu' metadata payload for inventory.
package hostgpuimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/hostgpu"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const flareFileName = "hostgpu.json"

// PCIVendorMap maps PCI vendor IDs to vendor names
var PCIVendorMap = map[string]string{
	"0x10de": "nvidia",
	"0x8086": "intel",
	"0x1002": "amd",
}

// Collector functions
var (
	baseGPUGet = collectBaseGPUInfo
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newGPUHostProvider))
}

type gpuDeviceMetadata struct {
	ID             int    `json:"gou_id"`
	Vendor         string `json:"gpu_vendor"`
	Device         string `json:"gpu_device"`
	DriverVersion  string `json:"driver_version"`
	RuntimeVersion string `json:"runtime_version"`
	UUID           string `json:"gpu_uuid"`
	Architecture   string `json:"gpu_architecture"`

	ComputeVersion string `json:"gpu_compute_version"` //e.g: in nvidia this refers to Compute Capability
	ProcessorUnits int    `json:"gpu_processor_units"` // e.g: in nvidia it is SMCount (Streaming MultiProcessor Count)
	Cores          int    `json:"gpu_cores"`           //e.g: for nvidia it is cuda cores

	TotalMemory       int64 `json:"device_total memory"`
	MaxClockRate      int   `json:"device_max_clock_rate"`
	MemoryClockRate   int   `json:"device_memory_clock_rate"`
	MemoryBusWidth    int   `json:"device_memory_bus_width"`
	L2CacheSize       int   `json:"device_l2_cache_size"`
	WarpSize          int   `json:"device_warp_size"`
	RegistersPerBlock int   `json:"device_registers_per_block"`
}

// hostGPUMetadata contains host's gpu metadata
type hostGPUMetadata struct {
	Devices []gpuDeviceMetadata `json:"devices"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  *hostGPUMetadata `json:"host_gpu_metadata"`
	UUID      string           `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories host gpu payload any more, payload is too big for intake")
}

// collectBaseGPUInfo collects basic GPU information available through filesystem
func collectBaseGPUInfo() (*hostGPUMetadata, error) {
	return nil, nil
}

// collectNvidiaGPUInfo enhances GPU information for NVIDIA devices using NVML
func collectNvidiaGPUInfo(device gpuDeviceMetadata) (*gpuDeviceMetadata, error) {
	return nil, nil
}

type gpuHost struct {
	util.InventoryPayload

	log      log.Component
	conf     config.Component
	data     *hostGPUMetadata
	hostname string
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
}

type provides struct {
	fx.Out

	Comp          hostgpu.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

func newGPUHostProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	gh := &gpuHost{
		conf:     deps.Config,
		log:      deps.Log,
		hostname: hname,
		data:     &hostGPUMetadata{},
	}
	gh.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, gh.getPayload, flareFileName)

	return provides{
		Comp:          gh,
		Provider:      gh.MetadataProvider(),
		FlareProvider: gh.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(gh.writePayloadAsJSON, "/metadata/inventory-host", "GET"),
	}
}

func (gh *gpuHost) fillData() {

	var err error
	gh.data, err = baseGPUGet()
	if err != nil {
		gh.log.Errorf("Failed to collect base GPU information: %v", err)
		return
	}

}

func (gh *gpuHost) getPayload() marshaler.JSONMarshaler {
	gh.fillData()

	return &Payload{
		Hostname:  gh.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  gh.data,
		UUID:      uuid.GetUUID(),
	}
}

func (gh *gpuHost) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := gh.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}
