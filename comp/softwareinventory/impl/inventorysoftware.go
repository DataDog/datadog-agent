// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package softwareinventoryimpl contains the implementation of the inventory software component.
// This package provides the concrete implementation of the inventory software component
// that collects software inventory data from the Windows system through the System Probe.
package softwareinventoryimpl

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	softwareinventory "github.com/DataDog/datadog-agent/comp/softwareinventory/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const flareFileName = "inventorysoftware.json"

// sysProbeClient is an interface for system probe used for dependency injection and testing.
// This interface abstracts the communication with the System Probe to retrieve software inventory data,
// allowing for easier testing and dependency injection.
type sysProbeClient interface {
	// GetCheck retrieves software inventory data from the specified System Probe module.
	// This method communicates with the System Probe to collect software information
	// from the Windows registry and other system sources.
	GetCheck(module types.ModuleName) ([]software.Entry, error)
}

// sysProbeClientWrapper wraps the real sysprobeclient.CheckClient to implement mockSysProbeClient.
// This wrapper provides a clean interface to the System Probe client while maintaining
// compatibility with the existing client implementation.
type sysProbeClientWrapper struct {
	// don't use this field directly, it's used for lazy initialization
	client *sysprobeclient.CheckClient
	// clientFn is used to lazily initialize the client
	clientFn func() *sysprobeclient.CheckClient
}

// GetCheck implements sysProbeClient.GetCheck by delegating to the wrapped client.
// It unmarshals the wire format (SoftwareInventoryWireEntry) and converts to []software.Entry.
func (w *sysProbeClientWrapper) GetCheck(module types.ModuleName) ([]software.Entry, error) {
	if w.client == nil {
		w.client = w.clientFn()
	}
	responses, err := sysprobeclient.GetCheck[[]software.SoftwareInventoryWireEntry](w.client, module)
	if err != nil {
		return nil, err
	}
	entries := make([]software.Entry, len(responses))
	for i := range responses {
		entries[i] = software.WireToEntry(&responses[i])
	}
	return entries, nil
}

// softwareInventory is the implementation of the Component interface.
// This struct holds the state and dependencies needed to collect and manage
// software inventory data from the Windows system.
type softwareInventory struct {
	// true if the component was enabled in the configuration
	enabled bool
	// log provides logging capabilities for the component
	log log.Component
	// sysProbeClient is used to communicate with the System Probe for data collection
	sysProbeClient sysProbeClient
	// cachedInventory stores the most recently collected software inventory data
	cachedInventory []software.Entry
	// cachedInventoryMu protects concurrent access to cachedInventory
	cachedInventoryMu sync.RWMutex
	// hostname identifies the system where the inventory was collected
	hostname string
	// eventPlatform provides access to the event platform forwarder
	eventPlatform eventplatform.Component
	// jitter is the time to wait before sending the first payload, in seconds
	jitter time.Duration
	// interval is the time to wait between payloads, in minutes
	interval time.Duration
	// sleepFunc is the function used for sleeping, can be overridden in tests
	sleepFunc func(time.Duration)
}

// Requires defines the dependencies required by the inventory software component.
// This struct defines all the required dependencies that must be provided
// when creating a new inventory software component instance.
type Requires struct {
	// Log provides logging capabilities for the component
	Log log.Component
	// Config provides access to the agent configuration
	Config config.Component
	// SysprobeConfig provides access to the system probe configuration
	SysprobeConfig sysprobeconfig.Component
	// Serializer is used to serialize and send data to the backend
	Serializer serializer.MetricSerializer
	// Hostname provides the hostname of the current system
	Hostname hostnameinterface.Component
	// EventPlatform provides access to the event platform forwarder
	EventPlatform eventplatform.Component
	// Provides lifecycle hooks for the component
	Lc compdef.Lifecycle
}

// Provides defines the output of the inventory software component.
// This struct defines all the services and providers that the component
// makes available to the rest of the system.
type Provides struct {
	// Comp is the main component interface for software inventory
	Comp softwareinventory.Component
	// FlareProvider provides software inventory data for flare collection
	FlareProvider flaretypes.Provider
	// StatusHeaderProvider provides status information for the agent status page
	StatusHeaderProvider status.HeaderInformationProvider
	// Endpoint provides HTTP endpoint access to software inventory data
	Endpoint api.AgentEndpointProvider
}

// New creates a new inventory software component with the default sysprobeclient
func New(reqs Requires) (Provides, error) {
	return newWithClient(reqs, &sysProbeClientWrapper{
		clientFn: func() *sysprobeclient.CheckClient {
			return sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(reqs.SysprobeConfig.GetString("system_probe_config.sysprobe_socket")))
		},
	}, time.Sleep)
}

// newWithClient creates a new inventory software component with a custom sysprobeclient and sleep function
func newWithClient(reqs Requires, client sysProbeClient, sleepFunc func(time.Duration)) (Provides, error) {
	hname, err := reqs.Hostname.Get(context.Background())
	if err != nil {
		return Provides{}, err
	}

	is := &softwareInventory{
		enabled:        reqs.Config.GetBool("software_inventory.enabled"),
		log:            reqs.Log,
		sysProbeClient: client,
		hostname:       hname,
		eventPlatform:  reqs.EventPlatform,
		sleepFunc:      sleepFunc,
	}

	if !is.enabled {
		return Provides{
			Comp: is,
		}, nil
	}

	localSource := rand.NewSource(time.Now().UnixNano())
	localRand := rand.New(localSource)

	is.jitter = time.Duration(localRand.Intn(max(reqs.Config.GetInt("software_inventory.jitter"), 60))) * time.Second
	is.interval = time.Duration(max(reqs.Config.GetInt("software_inventory.interval"), 10)) * time.Minute

	is.log.Infof("Starting the inventory software component")

	ctx, cancel := context.WithCancel(context.Background())
	reqs.Lc.Append(compdef.Hook{
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
	go is.startSoftwareInventoryCollection(ctx)

	return Provides{
		Comp:                 is,
		FlareProvider:        is.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(is),
		Endpoint:             api.NewAgentEndpointProvider(is.writePayloadAsJSON, "/metadata/software", "GET"),
	}, nil
}

func (is *softwareInventory) startSoftwareInventoryCollection(ctx context.Context) {
	// Wait for System Probe to be ready with simple retry loop
	for {
		initialInventory, err := is.sysProbeClient.GetCheck(sysconfig.SoftwareInventoryModule)
		if err == nil {
			is.log.Debug("Initial software inventory collection completed")
			is.cachedInventoryMu.Lock()
			is.cachedInventory = initialInventory
			is.cachedInventoryMu.Unlock()
			break
		}

		// Only retry if System Probe hasn't started yet.
		// This error is returned for the first 5min after the Agent startup (configurable with check_system_probe_startup_time).
		if !errors.Is(err, sysprobeclient.ErrNotStartedYet) {
			_ = is.log.Warnf("Initial software inventory collection failed, will retry on next interval: %v", err)
			break
		}

		is.log.Debugf("System Probe not ready yet, retrying in 10s: %v", err)

		// Use a timer that can be cancelled by context
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}

	// Send the initial collection with a random jitter
	is.log.Debugf("Sending initial software inventory collection with %v jitter", is.jitter)
	is.sleepFunc(is.jitter)

	// Always send the initial payload on start-up.
	// We'll send the follow-up payloads only on change.
	err := is.sendPayload()
	if err != nil {
		_ = is.log.Errorf("Failed to send software inventory: %v", err)
	}

	ticker := time.NewTicker(is.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			newInventory, err := is.sysProbeClient.GetCheck(sysconfig.SoftwareInventoryModule)
			if err != nil {
				_ = is.log.Warnf("Failed to get software inventory: %v", err)
				continue
			}

			// TODO: Compare old and new inventory

			is.cachedInventoryMu.Lock()
			is.cachedInventory = newInventory
			is.cachedInventoryMu.Unlock()

			err = is.sendPayload()
			if err != nil {
				_ = is.log.Errorf("Failed to send software inventory: %v", err)
				continue
			}

		case <-ctx.Done():
			return
		}
	}
}

func (is *softwareInventory) sendPayload() error {
	forwarder, ok := is.eventPlatform.Get()
	if !ok {
		return errors.New("event platform forwarder not available")
	}

	payload := is.getPayload()
	if payload == nil {
		// No cached inventory available, skip sending payload
		return nil
	}

	jsonPayload, err := payload.MarshalJSON()
	if err != nil {
		return err
	}

	msg := message.NewMessage(jsonPayload, nil, "", 0)

	// Send the message through the event platform
	if err = forwarder.SendEventPlatformEvent(msg, eventplatform.EventTypeSoftwareInventory); err != nil {
		return fmt.Errorf("error sending payload to event platform: %v", err)
	}
	return nil
}

// getPayload creates and returns a new software inventory payload.
// This method triggers a refresh of the cached data and returns a properly
// formatted payload for transmission to the backend.
func (is *softwareInventory) getPayload() marshaler.JSONMarshaler {
	is.cachedInventoryMu.RLock()
	defer is.cachedInventoryMu.RUnlock()

	if is.cachedInventory == nil {
		return nil
	}

	return &Payload{
		Hostname: is.hostname, // Set from the component's hostname field
		Metadata: HostSoftware{
			Software: is.cachedInventory,
		},
	}
}

// writePayloadAsJSON writes the software inventory payload as JSON to the HTTP response.
// This method is used by the HTTP endpoint to serve software inventory data
// in JSON format for external consumption.
func (is *softwareInventory) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	payload := is.getPayload()
	if payload == nil {
		httputils.SetJSONError(w, errors.New("software inventory data not yet available"), 503)
		return
	}

	json, err := payload.MarshalJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	_, _ = w.Write(json)
}

// FlareProvider returns a flare provider for the software inventory component
func (is *softwareInventory) FlareProvider() flaretypes.Provider {
	return flaretypes.NewProvider(
		func(fb flaretypes.FlareBuilder) error {
			payload := is.getPayload()
			if payload == nil {
				msg := "Software inventory data collection failed or returned no results"
				if !is.enabled {
					msg = "Software Inventory component is not enabled"
				}
				return fb.AddFile(flareFileName, []byte(msg))
			}
			return fb.AddFileFromFunc(flareFileName, payload.MarshalJSON)
		})
}
