// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package auditor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

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

func unmarshalRegistryV0(b []byte) (map[string]*RegistryEntry, error) {
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
