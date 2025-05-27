// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package inventorysoftware implements the inventory software component, to collect installed software inventory.
package inventorysoftware

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/status"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"go.uber.org/fx"

	types "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

const flareFileName = "inventorysoftware.json"

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(New))
}

// SoftwareInventoryMetadata represents the metadata for a software product
type SoftwareInventoryMetadata map[string]string

// SoftwareInventoryMap represents a mapping of product codes to their metadata
type SoftwareInventoryMap map[string]SoftwareInventoryMetadata


// SysProbeClient is an interface for the sysprobeclient used for dependency injection and testing.
type SysProbeClient interface {
	GetCheck(module types.ModuleName) (SoftwareInventoryMap, error)
}

// sysProbeClientWrapper wraps the real sysprobeclient.CheckClient to implement SysProbeClient.
type sysProbeClientWrapper struct {
	client *sysprobeclient.CheckClient
}

func (w *sysProbeClientWrapper) GetCheck(module types.ModuleName) (SoftwareInventoryMap, error) {
	return sysprobeclient.GetCheck[SoftwareInventoryMap](w.client, module)
}

// inventorySoftware is the implementation of the Component interface.
type inventorySoftware struct {
	util.InventoryPayload

	log             log.Component
	sysProbeClient  SysProbeClient
	cachedInventory []*SoftwareMetadata
}

// Dependencies is the dependencies for the inventory software component.
type Dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
}

// Provides defines the output of the hostgpu component
type Provides struct {
	fx.Out

	Comp                 Component
	Provider             runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
	Endpoint             api.AgentEndpointProvider
}

// NewWithClient creates a new inventory software component with a custom sysprobeclient
func NewWithClient(deps Dependencies, client SysProbeClient) Provides {
	if client == nil {
		client = &sysProbeClientWrapper{
			client: sysprobeclient.GetCheckClient(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
		}
	}
	is := &inventorySoftware{
		log:            deps.Log,
		sysProbeClient: client,
	}
	is.log.Infof("Starting the inventory software component")
	is.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, is.getPayload, flareFileName)
	return Provides{
		Comp:                 is,
		Provider:             is.InventoryPayload.MetadataProvider(),
		FlareProvider:        is.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(is),
		Endpoint:             api.NewAgentEndpointProvider(is.writePayloadAsJSON, "/metadata/software", "GET"),
	}
}

// New creates a new inventory software component with the default sysprobeclient
func New(deps Dependencies) Provides {
	return NewWithClient(deps, nil)
}

func (is *inventorySoftware) refreshCachedValues() error {
	is.log.Infof("Collecting Software Inventory")
	is.cachedInventory = nil

	installedSoftware, err := is.sysProbeClient.GetCheck(sysconfig.InventorySoftwareModule)
	if err != nil {
		return is.log.Errorf("error getting software inventory: %v", err)
	}

	for productCode, metadata := range installedSoftware {
		is.cachedInventory = append(is.cachedInventory, &SoftwareMetadata{
			ProductCode: productCode,
			Metadata:    metadata,
		})
	}

	return nil
}

func (is *inventorySoftware) getPayload() marshaler.JSONMarshaler {
	if err := is.refreshCachedValues(); err != nil {
		return nil
	}

	return &Payload{
		Metadata: is.cachedInventory,
	}
}

func (is *inventorySoftware) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	json, err := is.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	_, _ = w.Write(json)
}
