// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provider

import (
	"time"
)

type dummyCollector struct {
	id              string
	cStats          map[string]*ContainerStats
	cPIDStats       map[string]*ContainerPIDStats
	cOpenFilesCount map[string]*uint64
	cNetStats       map[string]*ContainerNetworkStats
	cIDForPID       map[int]string
	selfContainerID string
	err             error
}

func (d dummyCollector) ID() string {
	return d.id
}

func (d dummyCollector) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error) { //nolint:revive // TODO fix revive unused-parameter
	return d.cStats[containerNS+containerID], d.err
}

func (d dummyCollector) GetContainerPIDStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerPIDStats, error) { //nolint:revive // TODO fix revive unused-parameter
	return d.cPIDStats[containerNS+containerID], d.err
}

func (d dummyCollector) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) { //nolint:revive // TODO fix revive unused-parameter
	return d.cOpenFilesCount[containerNS+containerID], d.err
}

func (d dummyCollector) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) { //nolint:revive // TODO fix revive unused-parameter
	return d.cNetStats[containerNS+containerID], d.err
}

func (d dummyCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) { //nolint:revive // TODO fix revive unused-parameter
	return d.cIDForPID[pid], d.err
}

func (d dummyCollector) GetSelfContainerID() (string, error) {
	return d.selfContainerID, nil
}
