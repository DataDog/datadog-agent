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

type schedulerInterface interface {
	AddCollector(name string, interval time.Duration) error
	TriggerAndResetCollectorTimer(name string, delay time.Duration)
}

type autoConfigInterface interface {
	GetLoadedConfigs() map[string]integration.Config
}

type collectorInterface interface {
	GetAllInstanceIDs(checkName string) []check.ID
}

type checkMetadataCacheEntry struct {
	LastUpdated           time.Time
	CheckInstanceMetadata CheckInstanceMetadata
}

var (
	checkMetadataCache = make(map[string]*checkMetadataCacheEntry) // by check ID
	checkCacheMutex    = &sync.Mutex{}
	agentMetadataCache = make(AgentMetadata)
	agentCacheMutex    = &sync.Mutex{}

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

const (
	// CloudProviderMetatadaName is the field name to use to set the cloud
	// provider name in the agent metadata.
	CloudProviderMetatadaName = "cloud_provider"
)

// SetAgentMetadata updates the agent metadata value in the cache
func SetAgentMetadata(name string, value interface{}) {
	agentCacheMutex.Lock()
	defer agentCacheMutex.Unlock()

	if agentMetadataCache[name] != value {
		agentMetadataCache[name] = value

		select {
		case metadataUpdatedC <- nil:
		default: // To make sure this call is not blocking
		}
	}
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

	if entry.CheckInstanceMetadata[key] != value {
		entry.LastUpdated = timeNow()
		entry.CheckInstanceMetadata[key] = value

		select {
		case metadataUpdatedC <- nil:
		default: // To make sure this call is not blocking
		}
	}
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

	checkInstanceMetadata["last_updated"] = lastUpdated.UnixNano()
	checkInstanceMetadata["config.hash"] = checkID
	checkInstanceMetadata["config.provider"] = configProvider

	return &checkInstanceMetadata
}

// GetPayload fills and returns the check metadata payload
func GetPayload(hostname string, ac autoConfigInterface, coll collectorInterface) *Payload {
	lastGetPayloadMutex.Lock()
	lastGetPayload = timeNow()
	lastGetPayloadMutex.Unlock()

	checkCacheMutex.Lock()
	defer checkCacheMutex.Unlock()

	checkMetadata := make(CheckMetadata)

	newCheckMetadataCache := make(map[string]*checkMetadataCacheEntry)

	if ac != nil {
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
	// Creating a copy of agentMetadataCache
	agentMetadata := make(AgentMetadata)
	for k, v := range agentMetadataCache {
		agentMetadata[k] = v
	}

	return &Payload{
		Hostname:      hostname,
		Timestamp:     timeNow().UnixNano(),
		CheckMetadata: &checkMetadata,
		AgentMetadata: &agentMetadata,
	}
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
