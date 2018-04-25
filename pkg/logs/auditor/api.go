// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package auditor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// v2: In the third version of the auditor, we dropped Timestamp and used a generic Offset instead to reinforce the separation of concerns
// between the auditor and the log sources.

func (a *Auditor) unmarshalRegistryV2(b []byte) (map[string]*RegistryEntry, error) {
	var r JSONRegistry
	err := json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	registry := make(map[string]*RegistryEntry)
	for identifier, entry := range r.Registry {
		newEntry := entry
		registry[identifier] = &newEntry
	}
	return registry, nil
}

// v1: In the second version of the auditor, Timestamp became LastUpdated and we added Timestamp to record container offsets.

type registryEntryV1 struct {
	Timestamp   string
	Offset      int64
	LastUpdated time.Time
}

type jsonRegistryV1 struct {
	Version  int
	Registry map[string]registryEntryV1
}

func (a *Auditor) unmarshalRegistryV1(b []byte) (map[string]*RegistryEntry, error) {
	var r jsonRegistryV1
	err := json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	registry := make(map[string]*RegistryEntry)
	for identifier, entry := range r.Registry {
		switch {
		case entry.Offset > 0:
			registry[identifier] = &RegistryEntry{LastUpdated: entry.LastUpdated, Offset: strconv.FormatInt(entry.Offset, 10)}
		case entry.Timestamp != "":
			registry[identifier] = &RegistryEntry{LastUpdated: entry.LastUpdated, Offset: entry.Timestamp}
		default:
			// no valid offset for this entry
		}
	}
	return registry, nil
}

// v0: In the first version of the auditor, we were only recording file offsets

type registryEntryV0 struct {
	Path      string
	Timestamp time.Time
	Offset    int64
}

type jsonRegistryV0 struct {
	Version  int
	Registry map[string]registryEntryV0
}

func (a *Auditor) unmarshalRegistryV0(b []byte) (map[string]*RegistryEntry, error) {
	var r jsonRegistryV0
	err := json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	registry := make(map[string]*RegistryEntry)
	for identifier, entry := range r.Registry {
		switch {
		case entry.Offset > 0:
			// from v0 to v1 and further, we also prefixed path with file:
			newIdentifier := fmt.Sprintf("file:%s", identifier)
			registry[newIdentifier] = &RegistryEntry{LastUpdated: entry.Timestamp, Offset: strconv.FormatInt(entry.Offset, 10)}
		default:
			// no valid offset for this entry
		}
	}
	return registry, nil
}
