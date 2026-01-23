// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostgpuimpl implements a component to generate the 'host_gpu' metadata payload for inventory.
package hostgpuimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	hostgpu "github.com/DataDog/datadog-agent/comp/metadata/hostgpu/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const flareFileName = "hostgpu.json"

type gpuDeviceMetadata struct {
	Index              int    `json:"gpu_index"`
	Vendor             string `json:"gpu_vendor"`
	Name               string `json:"gpu_device"`
	DriverVersion      string `json:"gpu_driver_version"`
	UUID               string `json:"gpu_uuid"`
	Architecture       string `json:"gpu_architecture"`
	GPUType            string `json:"gpu_type"`
	SlicingMode        string `json:"gpu_slicing_mode"`
	VirtualizationMode string `json:"gpu_virtualization_mode"`
	ComputeVersion     string `json:"gpu_compute_version"` //e.g: in nvidia this refers to Compute Capability
	TotalCores         int    `json:"gpu_total_cores"`
	ParentGPUUUID      string `json:"gpu_parent_uuid"`
	//Total device memory in bytes
	TotalMemory        uint64 `json:"device_total_memory"`
	MaxSMClockRate     uint32 `json:"device_max_sm_clock_rate"`
	MaxMemoryClockRate uint32 `json:"device_max_memory_clock_rate"`
	MemoryBusWidth     uint32 `json:"device_memory_bus_width"`
}

// hostGPUMetadata contains host's gpu metadata
type hostGPUMetadata struct {
	Devices []*gpuDeviceMetadata `json:"devices"`
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

type gpuHost struct {
	util.InventoryPayload

	log      log.Component
	conf     config.Component
	wmeta    workloadmeta.Component
	data     *hostGPUMetadata
	hostname string
}

// Requires defines the dependencies for the hostgpu component
type Requires struct {
	WMeta      workloadmeta.Component
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

// Provides defines the output of the hostgpu component
type Provides struct {
	Comp          hostgpu.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

// NewGPUHostProvider creates a new hostgpu component
func NewGPUHostProvider(deps Requires) Provides {
	hname, _ := deps.Hostname.Get(context.Background())
	gh := &gpuHost{
		conf:     deps.Config,
		log:      deps.Log,
		wmeta:    deps.WMeta,
		hostname: hname,
		data:     &hostGPUMetadata{},
	}
	gh.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, gh.getPayload, flareFileName)

	return Provides{
		Comp:          gh,
		Provider:      gh.MetadataProvider(),
		FlareProvider: gh.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(gh.writePayloadAsJSON, "/metadata/host-gpu", "GET"),
	}
}

func (gh *gpuHost) fillData() {
	gpus := gh.wmeta.ListGPUs()
	if len(gpus) == 0 {
		gh.data = &hostGPUMetadata{}
		return
	}

	gh.data = &hostGPUMetadata{
		Devices: make([]*gpuDeviceMetadata, 0, len(gpus)),
	}
	for _, gpu := range gpus {
		dev := &gpuDeviceMetadata{
			Index:              gpu.Index,
			UUID:               gpu.ID,
			Vendor:             strings.ToLower(gpu.Vendor),
			DriverVersion:      gpu.DriverVersion,
			ComputeVersion:     gpu.ComputeCapability.String(),
			Name:               gpu.Name,
			Architecture:       gpu.Architecture,
			TotalCores:         gpu.TotalCores,
			TotalMemory:        gpu.TotalMemory,
			MemoryBusWidth:     gpu.MemoryBusWidth,
			GPUType:            gpu.GPUType,
			SlicingMode:        gpu.SlicingMode(),
			VirtualizationMode: gpu.VirtualizationMode,
			ParentGPUUUID:      gpu.ParentGPUUUID,
			MaxSMClockRate:     gpu.MaxClockRates[workloadmeta.GPUSM],
			MaxMemoryClockRate: gpu.MaxClockRates[workloadmeta.GPUMemory],
		}
		gh.data.Devices = append(gh.data.Devices, dev)
	}
}

func (gh *gpuHost) getPayload() marshaler.JSONMarshaler {
	gh.fillData()

	if len(gh.data.Devices) == 0 {
		return nil
	}

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
