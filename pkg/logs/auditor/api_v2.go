// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditor

// v2: In the third version of the auditor, we dropped Timestamp and used a generic Offset instead to reinforce the separation of concerns
// between the auditor and log sources.

func unmarshalRegistryV2(b []byte) (map[string]*RegistryEntry, error) {
	panic("not called")
}
