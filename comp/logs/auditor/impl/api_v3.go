// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditorimpl

import (
	"encoding/json"
)

// v3: In the fourth version of the auditor, we replaced FilePath with Fingerprint to handle experimental fingerprinting.

func unmarshalRegistryV3(b []byte) (map[string]*RegistryEntry, error) {
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
