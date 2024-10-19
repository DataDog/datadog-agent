// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && test

package servicediscovery

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
)

const (
	dummyContainerID = "abcd"
)

type containerProviderStub struct {
	pidToCid map[int]string
}

func newContainerProviderStub(targetPIDs []int) proccontainers.ContainerProvider {
	pidToCid := make(map[int]string)

	for _, pid := range targetPIDs {
		pidToCid[pid] = dummyContainerID
	}

	return &containerProviderStub{
		pidToCid: pidToCid,
	}
}

func (*containerProviderStub) GetContainers(_ time.Duration, _ map[string]*proccontainers.ContainerRateMetrics) ([]*model.Container, map[string]*proccontainers.ContainerRateMetrics, map[int]string, error) {
	return nil, nil, nil, nil
}

func (s *containerProviderStub) GetPidToCid(_ time.Duration) map[int]string {
	return s.pidToCid
}
