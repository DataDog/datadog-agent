// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package checkmetadata

import "sync"

// metadata name -> value
type instanceMetadataCache map[string]string

var (
	// checkMetadataCache maps checkID -> instanceMetadataCache
	// The checkID is necessary since different instances may
	// report the same metadata name with different values.
	checkMetadataCache = make(map[string]instanceMetadataCache)
	cacheMutex         = &sync.Mutex{}
)

// SetCheckMetadata updates a metadata value for one check instance in the cache.
func SetCheckMetadata(checkID, name, value string) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	_, found := checkMetadataCache[checkID]
	if !found {
		checkMetadataCache[checkID] = make(instanceMetadataCache)
	}

	checkMetadataCache[checkID][name] = value
}

// GetPayload fills and returns the check metadata payload
func GetPayload() *Payload {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	payload := Payload{}
	for _, instanceCache := range checkMetadataCache {
		for name, value := range instanceCache {
			payload = append(payload, [2]string{name, value})
		}
	}

	// clear the cache
	checkMetadataCache = make(map[string]instanceMetadataCache)
	return &payload
}
