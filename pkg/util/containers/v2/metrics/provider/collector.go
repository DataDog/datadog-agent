// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import "time"

// Collector defines an interface allowing to get stats from a containerID.
// All implementations should allow for concurrent access.
type Collector interface {
	ID() string
	GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error)
	GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error)
	// OpenFiles count has been splitted of as it incurs a high performance penalty on Linux
	GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error)
	MetaCollector
}
