// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditor

import (
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
	panic("not called")
}
