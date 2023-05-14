// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditor

import (
	"encoding/json"
	"strconv"
	"time"
)

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

func unmarshalRegistryV1(b []byte) (map[string]*RegistryEntry, error) {
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
