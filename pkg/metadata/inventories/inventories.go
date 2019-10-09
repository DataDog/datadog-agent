// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package inventories

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

type checkMetadataCacheEntry struct {
	LastUpdated           int64
	CheckInstanceMetadata CheckInstanceMetadata
}

// getAllInstanceIDsInterface is a simplify interface for the collector that
// ease testing
type getAllInstanceIDsInterface interface {
	GetAllInstanceIDs(checkName string) []check.ID
}

// getLoadedConfigsInterface is a simplify interface for autodiscovery that
// ease testing
type getLoadedConfigsInterface interface {
	GetLoadedConfigs() map[string]integration.Config
}

var (
	// For testing purposes
	nowNano = func() int64 { return time.Now().UnixNano() }
)

var (
	checkMetadataCache = make(map[string]*checkMetadataCacheEntry) // by check ID
	checkCacheMutex    = &sync.Mutex{}
	agentMetadataCache = make(AgentMetadata)
	agentCacheMutex    = &sync.Mutex{}

	agentStartupTime = nowNano()

	// CloudProviderMetatadaName is the field name to use to set the cloud
	// provider name in the agent metadata.
	CloudProviderMetatadaName = "cloud_provider"
)

// SetAgentMetadata updates the agent metadata value in the cache
func SetAgentMetadata(name string, value interface{}) {
	agentCacheMutex.Lock()
	defer agentCacheMutex.Unlock()

	agentMetadataCache[name] = value
}

// SetCheckMetadata updates a metadata value for one check instance in the cache.
func SetCheckMetadata(checkID, key string, value interface{}) {
	checkCacheMutex.Lock()
	defer checkCacheMutex.Unlock()

	entry, found := checkMetadataCache[checkID]
	if !found {
		entry = &checkMetadataCacheEntry{
			CheckInstanceMetadata: make(CheckInstanceMetadata),
		}
		checkMetadataCache[checkID] = entry
	}

	entry.LastUpdated = nowNano()
	entry.CheckInstanceMetadata[key] = value
}

func getCheckInstanceMetadata(checkID, configProvider string) *CheckInstanceMetadata {

	var checkInstanceMetadata CheckInstanceMetadata
	lastUpdated := agentStartupTime

	entry, found := checkMetadataCache[checkID]
	if found {
		checkInstanceMetadata = entry.CheckInstanceMetadata
		lastUpdated = entry.LastUpdated
	} else {
		checkInstanceMetadata = make(CheckInstanceMetadata)
	}

	checkInstanceMetadata["last_updated"] = lastUpdated
	checkInstanceMetadata["config.hash"] = checkID
	checkInstanceMetadata["config.provider"] = configProvider

	return &checkInstanceMetadata
}

// GetPayload fills and returns the check metadata payload
func GetPayload(hostname string, ac getLoadedConfigsInterface, coll getAllInstanceIDsInterface) *Payload {
	checkCacheMutex.Lock()
	defer checkCacheMutex.Unlock()

	checkMetadata := make(CheckMetadata)

	newCheckMetadataCache := make(map[string]*checkMetadataCacheEntry)

	if ac != nil && coll != nil {
		configs := ac.GetLoadedConfigs()
		for _, config := range configs {
			checkMetadata[config.Name] = make([]*CheckInstanceMetadata, 0)
			instanceIDs := coll.GetAllInstanceIDs(config.Name)
			for _, id := range instanceIDs {
				checkInstanceMetadata := getCheckInstanceMetadata(string(id), config.Provider)
				checkMetadata[config.Name] = append(checkMetadata[config.Name], checkInstanceMetadata)
				if entry, found := checkMetadataCache[string(id)]; found {
					newCheckMetadataCache[string(id)] = entry
				}
			}
		}
	}

	// newCheckMetadataCache only contains checks that are still running, this way it doesn't grow forever
	checkMetadataCache = newCheckMetadataCache

	agentCacheMutex.Lock()
	defer agentCacheMutex.Unlock()

	return &Payload{
		Hostname:      hostname,
		Timestamp:     nowNano(),
		CheckMetadata: &checkMetadata,
		AgentMetadata: &agentMetadataCache,
	}
}
