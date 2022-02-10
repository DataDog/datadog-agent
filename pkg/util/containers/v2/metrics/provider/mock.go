// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provider

import (
	"time"
)

type dummyCollector struct {
	id        string
	cStats    map[string]*ContainerStats
	cNetStats map[string]*ContainerNetworkStats
	err       error
}

func (d dummyCollector) ID() string {
	return d.id
}

func (d dummyCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	return d.cStats[containerID], d.err
}

func (d dummyCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	return d.cNetStats[containerID], d.err
}
