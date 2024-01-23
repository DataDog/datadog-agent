// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package auditor

import (
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
	panic("not called")
}
