// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri
// +build cri

package cri

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri/crimock"
)

func TestGetContainerStats(t *testing.T) {
	containerID := "123"

	mockedCriClient := new(crimock.MockCRIClient)
	mockedCriClient.On("GetContainerStats", containerID).Return(
		&pb.ContainerStats{
			Attributes: &pb.ContainerAttributes{
				Id: containerID,
			},
			Cpu: &pb.CpuUsage{
				UsageCoreNanoSeconds: &pb.UInt64Value{
					Value: 1000,
				},
			},
			Memory: &pb.MemoryUsage{
				WorkingSetBytes: &pb.UInt64Value{
					Value: 1024,
				},
			},
		},
		nil,
	)

	collector := criCollector{
		client: mockedCriClient,
	}

	stats, err := collector.GetContainerStats(containerID, 10*time.Second)
	assert.NoError(t, err)

	assert.Equal(t, util.UIntToFloatPtr(1000), stats.CPU.Total)
	assert.Equal(t, util.UIntToFloatPtr(1024), stats.Memory.RSS)
}

func TestGetContainerNetworkStats(t *testing.T) {
	collector := criCollector{}
	stats, err := collector.GetContainerNetworkStats("123", time.Second)
	assert.NoError(t, err)
	assert.Nil(t, stats) // The CRI collector does not return any network data
}
