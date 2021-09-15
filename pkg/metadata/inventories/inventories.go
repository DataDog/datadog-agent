// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventories

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type schedulerInterface interface {
	TriggerAndResetCollectorTimer(name string, delay time.Duration)
}

// AutoConfigInterface is an interface for the GetLoadedConfigs method of autodiscovery
type AutoConfigInterface interface {
	GetLoadedConfigs() map[string]integration.Config
}

// CollectorInterface is an interface for the GetAllInstanceIDs method of the collector
type CollectorInterface interface {
	GetAllInstanceIDs(checkName string) []check.ID
}

type checkMetadataCacheEntry struct {
	LastUpdated           time.Time
	CheckInstanceMetadata CheckInstanceMetadata
}

var (
	checkMetadata      = make(map[string]*checkMetadataCacheEntry) // by check ID
	checkMetadataMutex = &sync.Mutex{}
	agentMetadata      = make(AgentMetadata)
	agentMetadataMutex = &sync.Mutex{}

	agentStartupTime = timeNow()

	lastGetPayload      = timeNow()
	lastGetPayloadMutex = &sync.Mutex{}

	metadataUpdatedC = make(chan interface{}, 1)
)

var (
	// For testing purposes
	timeNow   = time.Now
	timeSince = time.Since
)

// AgentMetadataName is an enum type containing all defined keys for
// SetAgentMetadata.
type AgentMetadataName string

// Constants for the metadata names; these are defined in
// pkg/metadata/inventories/README.md and any additions should
// be updated there as well.
const (
	AgentCloudProvider      AgentMetadataName = "cloud_provider"
	AgentHostnameSource     AgentMetadataName = "hostname_source"
	AgentVersion            AgentMetadataName = "version"
	AgentFlavor             AgentMetadataName = "flavor"
	AgentInstallerVersion   AgentMetadataName = "install_method_installer_version"
	AgentInstallTool        AgentMetadataName = "install_method_tool"
	AgentInstallToolVersion AgentMetadataName = "install_method_tool_version"
)

// SetAgentMetadata updates the agent metadata value in the cache
func SetAgentMetadata(name AgentMetadataName, value interface{}) {
	agentMetadataMutex.Lock()
	defer agentMetadataMutex.Unlock()

	if agentMetadata[string(name)] != value {
		agentMetadata[string(name)] = value

		select {
		case metadataUpdatedC <- nil:
		default: // To make sure this call is not blocking
		}
	}
}

// SetCheckMetadata updates a metadata value for one check instance in the cache.
func SetCheckMetadata(checkID, key string, value interface{}) {
	checkMetadataMutex.Lock()
	defer checkMetadataMutex.Unlock()

	entry, found := checkMetadata[checkID]
	if !found {
		entry = &checkMetadataCacheEntry{
			CheckInstanceMetadata: make(CheckInstanceMetadata),
		}
		checkMetadata[checkID] = entry
	}

	if entry.CheckInstanceMetadata[key] != value {
		entry.LastUpdated = timeNow()
		entry.CheckInstanceMetadata[key] = value

		select {
		case metadataUpdatedC <- nil:
		default: // To make sure this call is not blocking
		}
	}
}

func createCheckInstanceMetadata(checkID, configProvider string) *CheckInstanceMetadata {
	const transientFields = 3

	var checkInstanceMetadata CheckInstanceMetadata
	var lastUpdated time.Time

	if entry, found := checkMetadata[checkID]; found {
		checkInstanceMetadata = make(CheckInstanceMetadata, len(entry.CheckInstanceMetadata)+transientFields)
		for k, v := range entry.CheckInstanceMetadata {
			checkInstanceMetadata[k] = v
		}
		lastUpdated = entry.LastUpdated
	} else {
		checkInstanceMetadata = make(CheckInstanceMetadata, transientFields)
		lastUpdated = agentStartupTime
	}

	checkInstanceMetadata["last_updated"] = lastUpdated.UnixNano()
	checkInstanceMetadata["config.hash"] = checkID
	checkInstanceMetadata["config.provider"] = configProvider

	return &checkInstanceMetadata
}

// CreatePayload fills and returns the inventory metadata payload
func CreatePayload(ctx context.Context, hostname string, ac AutoConfigInterface, coll CollectorInterface) *Payload {
	checkMetadataMutex.Lock()
	defer checkMetadataMutex.Unlock()

	// Collect check metadata for the payload
	payloadCheckMeta := make(CheckMetadata)

	foundInCollector := map[string]struct{}{}
	if ac != nil {
		configs := ac.GetLoadedConfigs()
		for _, config := range configs {
			payloadCheckMeta[config.Name] = make([]*CheckInstanceMetadata, 0)
			instanceIDs := coll.GetAllInstanceIDs(config.Name)
			for _, id := range instanceIDs {
				checkInstanceMetadata := createCheckInstanceMetadata(string(id), config.Provider)
				payloadCheckMeta[config.Name] = append(payloadCheckMeta[config.Name], checkInstanceMetadata)
				foundInCollector[string(id)] = struct{}{}
			}
		}
	}
	// if metadata were added for a check not in the collector we still need
	// to add them to the payloadCheckMeta (this happens when using the
	// 'check' command)
	for id := range checkMetadata {
		if _, found := foundInCollector[id]; !found {
			// id should be "check_name:check_hash"
			parts := strings.SplitN(id, ":", 2)
			payloadCheckMeta[parts[0]] = append(payloadCheckMeta[parts[0]], createCheckInstanceMetadata(id, ""))
		}
	}

	agentMetadataMutex.Lock()
	defer agentMetadataMutex.Unlock()

	// Create a static copy of agentMetadata for the payload
	payloadAgentMeta := make(AgentMetadata)
	for k, v := range agentMetadata {
		payloadAgentMeta[k] = v
	}

	return &Payload{
		Hostname:      hostname,
		Timestamp:     timeNow().UnixNano(),
		CheckMetadata: &payloadCheckMeta,
		AgentMetadata: &payloadAgentMeta,
	}
}

// GetPayload returns a new inventory metadata payload and updates lastGetPayload
func GetPayload(ctx context.Context, hostname string, ac AutoConfigInterface, coll CollectorInterface) *Payload {
	lastGetPayloadMutex.Lock()
	defer lastGetPayloadMutex.Unlock()
	lastGetPayload = timeNow()

	return CreatePayload(ctx, hostname, ac, coll)
}

// StartMetadataUpdatedGoroutine starts a routine that listens to the metadataUpdatedC
// signal to run the collector out of its regular interval.
func StartMetadataUpdatedGoroutine(sc schedulerInterface, minSendInterval time.Duration) error {
	go func() {
		for {
			<-metadataUpdatedC
			lastGetPayloadMutex.Lock()
			delay := minSendInterval - timeSince(lastGetPayload)
			if delay < 0 {
				delay = 0
			}
			sc.TriggerAndResetCollectorTimer("inventories", delay)
			lastGetPayloadMutex.Unlock()
		}
	}()
	return nil
}

// InitializeData inits the inventories payload with basic and static information (agent version, flavor name, ...)
func InitializeData() {
	SetAgentMetadata(AgentVersion, version.AgentVersion)
	SetAgentMetadata(AgentFlavor, flavor.GetFlavor())
}
