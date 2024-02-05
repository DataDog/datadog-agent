// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package uuid provides a way to generate and cache a UUID for the host.
// It leverages gopsutil for Linux and the machine GUID for Windows.
package uuid

import "github.com/DataDog/datadog-agent/pkg/util/cache"

// GetUUID returns the UUID of the host
var GetUUID = getUUID // For testing purposes
var guidCacheKey = cache.BuildAgentKey("host", "utils", "uuid")
