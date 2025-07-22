// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventorysoftwareimpl contains the implementation of the inventory software component.
// This package provides the concrete implementation of the inventory software component
// that collects software inventory data from the Windows system through the System Probe.
package inventorysoftwareimpl

import (
	"context"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"net/http"
	"time"

	inventorysoftware "github.com/DataDog/datadog-agent/comp/metadata/inventorysoftware/def"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/status"

	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const flareFileName = "inventorysoftware.json"

// SysProbeClient is an interface for the sysprobeclient used for dependency injection and testing.
// This interface abstracts the communication with the System Probe to retrieve software inventory data,
// allowing for easier testing and dependency injection.
type SysProbeClient interface {
	// GetCheck retrieves software inventory data from the specified System Probe module.
	// This method communicates with the System Probe to collect software information
	// from the Windows registry and other system sources.
	GetCheck(module types.ModuleName) ([]software.Entry, error)
}

// sysProbeClientWrapper wraps the real sysprobeclient.CheckClient to implement SysProbeClient.
// This wrapper provides a clean interface to the System Probe client while maintaining
// compatibility with the existing client implementation.
type sysProbeClientWrapper struct {
	client *sysprobeclient.CheckClient
}

// GetCheck implements SysProbeClient.GetCheck by delegating to the wrapped client.
// This method uses the generic GetCheck function to retrieve software inventory data
// from the System Probe with proper type safety.
func (w *sysProbeClientWrapper) GetCheck(module types.ModuleName) ([]software.Entry, error) {
	return sysprobeclient.GetCheck[[]software.Entry](w.client, module)
}

// inventorySoftware is the implementation of the Component interface.
// This struct holds the state and dependencies needed to collect and manage
// software inventory data from the Windows system.
type inventorySoftware struct {
	util.InventoryPayload

	// log provides logging capabilities for the component
	log log.Component
	// sysProbeClient is used to communicate with the System Probe for data collection
	sysProbeClient SysProbeClient
	// cachedInventory stores the most recently collected software inventory data
	cachedInventory []software.Entry
	// hostname identifies the system where the inventory was collected
	hostname string
	// enabled indicates whether software inventory collection is enabled in the configuration
	enabled bool
}

// Requires defines the dependencies required by the inventory software component.
// This struct defines all the required dependencies that must be provided
// when creating a new inventory software component instance.
type Requires struct {
	// Log provides logging capabilities for the component
	Log log.Component
	// Config provides access to the agent configuration
	Config config.Component
	// Serializer is used to serialize and send data to the backend
	Serializer serializer.MetricSerializer
	// Hostname provides the hostname of the current system
	Hostname hostnameinterface.Component
}

// Provides defines the output of the inventory software component.
// This struct defines all the services and providers that the component
// makes available to the rest of the system.
type Provides struct {
	// Comp is the main component interface for software inventory
	Comp inventorysoftware.Component
	// Provider is the metadata provider for software inventory data
	Provider runnerimpl.Provider
	// FlareProvider provides software inventory data for flare collection
	FlareProvider flaretypes.Provider
	// StatusHeaderProvider provides status information for the agent status page
	StatusHeaderProvider status.HeaderInformationProvider
	// Endpoint provides HTTP endpoint access to software inventory data
	Endpoint api.AgentEndpointProvider
}

// New creates a new inventory software component with the default sysprobeclient
func New(reqs Requires) (Provides, error) {
	return NewWithClient(reqs, nil)
}

// NewWithClient creates a new inventory software component with a custom sysprobeclient
func NewWithClient(reqs Requires, client SysProbeClient) (Provides, error) {
	if client == nil {
		client = &sysProbeClientWrapper{
			client: sysprobeclient.GetCheckClient(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
		}
	}
	hname, _ := reqs.Hostname.Get(context.Background())
	is := &inventorySoftware{
		log:            reqs.Log,
		sysProbeClient: client,
		hostname:       hname,
		enabled:        reqs.Config.GetBool("software_inventory.enabled"),
	}
	// Always provide the component because FX dependency injection expects
	// status providers, metadata providers, etc. to be available, not wrapped
	// in option.Option. The enabled flag only controls whether we call the System
	// Probe software inventory endpoint to collect actual data.
	// Note that there is a second way to disable this feature, through InventoryPayload.Enabled.
	// 'enable_metadata_collection' and 'inventories_enabled' both need to be set to true.
	is.log.Infof("Starting the inventory software component")
	is.InventoryPayload = util.CreateInventoryPayload(reqs.Config, reqs.Log, reqs.Serializer, is.getPayload, flareFileName)
	return Provides{
		Comp:                 is,
		Provider:             is.InventoryPayload.MetadataProvider(),
		FlareProvider:        is.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(is),
		Endpoint:             api.NewAgentEndpointProvider(is.writePayloadAsJSON, "/metadata/software", "GET"),
	}, nil
}

// refreshCachedValues updates the cached software inventory data by collecting
// fresh data from the System Probe. This method respects the enabled flag
// and will skip collection if the feature is disabled in the configuration.
func (is *inventorySoftware) refreshCachedValues() error {
	if !is.enabled {
		is.log.Debugf("Software inventory is disabled in agent configuration")
		return nil
	}
	is.log.Infof("Collecting Software Inventory")

	installedSoftware, err := is.sysProbeClient.GetCheck(sysconfig.InventorySoftwareModule)
	if err != nil {
		return is.log.Errorf("error getting software inventory: %v", err)
	}

	is.cachedInventory = installedSoftware

	return nil
}

// getPayload creates and returns a new software inventory payload.
// This method triggers a refresh of the cached data and returns a properly
// formatted payload for transmission to the backend.
func (is *inventorySoftware) getPayload() marshaler.JSONMarshaler {
	if err := is.refreshCachedValues(); err != nil {
		return nil
	}

	return &Payload{
		Hostname:  is.hostname,           // Set from the component's hostname field
		Timestamp: time.Now().UnixNano(), // Set to current time (nanoseconds)
		Metadata: HostSoftware{
			Software: is.cachedInventory,
		},
	}
}

// writePayloadAsJSON writes the software inventory payload as JSON to the HTTP response.
// This method is used by the HTTP endpoint to serve software inventory data
// in JSON format for external consumption.
func (is *inventorySoftware) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	json, err := is.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	_, _ = w.Write(json)
}
