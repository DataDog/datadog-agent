// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package inventories

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
)

type checkMetadataCacheEntry struct {
	LastUpdated           int64
	CheckInstanceMetadata CheckInstanceMetadata
}

var (
	// For testing purposes
	getLoadedConfigs  = common.AC.GetLoadedConfigs
	getAllInstanceIDs = common.Coll.GetAllInstanceIDs
	nowNano           = func() int64 { return time.Now().UnixNano() }

	checkMetadataCache = make(map[string]*checkMetadataCacheEntry) // by check ID
	checkCacheMutex    = &sync.Mutex{}
	agentMetadataCache = make(AgentMetadata)
	agentCacheMutex    = &sync.Mutex{}

	agentStartupTime = nowNano()
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
func GetPayload() *Payload {
	checkCacheMutex.Lock()
	defer checkCacheMutex.Unlock()

	checkMetadata := make(CheckMetadata)

	newCheckMetadataCache := make(map[string]*checkMetadataCacheEntry)

	configs := getLoadedConfigs()
	for _, config := range configs {
		checkMetadata[config.Name] = make([]*CheckInstanceMetadata, 0)
		instanceIDs := getAllInstanceIDs(config.Name)
		for _, id := range instanceIDs {
			checkInstanceMetadata := getCheckInstanceMetadata(string(id), config.Provider)
			checkMetadata[config.Name] = append(checkMetadata[config.Name], checkInstanceMetadata)
			if entry, found := checkMetadataCache[string(id)]; found {
				newCheckMetadataCache[string(id)] = entry
			}
		}
	}

	// newCheckMetadataCache only contains checks that are still running, this way it doesn't grow forever
	checkMetadataCache = newCheckMetadataCache

	agentCacheMutex.Lock()
	defer agentCacheMutex.Unlock()

	return &Payload{
		Timestamp:     nowNano(),
		CheckMetadata: &checkMetadata,
		AgentMetadata: &agentMetadataCache,
	}
}
