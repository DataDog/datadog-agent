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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newGPUHostProvider))
}

type gpuDeviceMetadata struct {
	Vendor            string
	Device            string
	Architecture      string
	ComputeCapability workloadmeta.GPUComputeCapability
	SMCount           int
	MigEnabled        bool
}

// hostGPUMetadata contains host's gpu metadata
type hostGPUMetadata struct {
	devices []gpuDeviceMetadata
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
//
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories host payload any more, payload is too big for intake")
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
	ih := &gpuHost{
		conf:     deps.Config,
		log:      deps.Log,
		hostname: hname,
		data:     &hostGPUMetadata{},
	}
	ih.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, ih.getPayload, "host.json")

	return provides{
		Comp:          ih,
		Provider:      ih.MetadataProvider(),
		FlareProvider: ih.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(ih.writePayloadAsJSON, "/metadata/inventory-host", "GET"),
	}
}

func (ih *gpuHost) fillData() {
	//TODO:
}

func (ih *gpuHost) getPayload() marshaler.JSONMarshaler {
	ih.fillData()

	return &Payload{
		Hostname:  ih.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  ih.data,
		UUID:      uuid.GetUUID(),
	}
}

func (ih *gpuHost) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := ih.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}
